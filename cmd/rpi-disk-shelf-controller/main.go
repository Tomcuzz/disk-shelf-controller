package main

import (
	"encoding/json"
	"fmt"
	"log"
	"os"
	"os/exec"
	"string"
	"time"

	"net/http"

	mqtt "github.com/eclipse/paho.mqtt.golang"
	"github.com/warthog618/gpiod"

	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/collectors"
	"github.com/prometheus/client_golang/prometheus/promauto"
	"github.com/prometheus/client_golang/prometheus/promhttp"
)

type metrics struct {
	onState prometheus.Gauge
}

var (
	mqttBroker    = os.Getenv("MQTT_BROKER")
	mqttClientID  = os.Getenv("MQTT_CLIENT_ID")
	mqttUsername  = os.Getenv("MQTT_USERNAME")
	mqttPassword  = os.Getenv("MQTT_PASSWORD")
	statusPinName = os.Getenv("STATUS_PIN") // Expects integer string like "17"
	togglePinName = os.Getenv("TOGGLE_PIN") // Expects integer string like "27"
	mountCommand  = os.Getenv("MOUNT_COMMAND")
	statusLine    *gpiod.Line
	toggleLine    *gpiod.Line
	metricAddr    = os.Getenv("listen-address")
	promMetrics   *metrics
	//var addr = flag.String("listen-address", ":8080", "The address to listen on for HTTP requests.")
)

const (
	haDiscoveryPrefix = "homeassistant"
	switchName        = "Disk Shelf"
	switchUniqueID    = "disk_shelf_switch"
	stateTopic        = "rpi-disk-shelf-controller/switch/state"
	commandTopic      = "rpi-disk-shelf-controller/switch/command"
	availabilityTopic = "rpi-disk-shelf-controller/status"
)

func main() {
	log.Println("Starting RPI Disk Shelf Controller")

	//Process pin names
	statusPinName = strings.Replace(statusPinName, "GPIO", "", -1)
	togglePinName = strings.Replace(togglePinName, "GPIO", "", -1)

	// Setup Metrics
	log.Println("Setting up metrics")
	if len(metricAddr) == 0 {
		metricAddr = ":8080"
	}
	reg := prometheus.NewRegistry()
	promMetrics = &metrics{
		onState: promauto.With(reg).NewGauge(prometheus.GaugeOpts{
			Name: "disk_shelf_on_state",
			Help: "The current on/off state of the disk shelf",
		}),
	}
	reg.MustRegister(
		collectors.NewGoCollector(),
		collectors.NewProcessCollector(collectors.ProcessCollectorOpts{}),
	)
	http.Handle("/metrics", promhttp.HandlerFor(reg, promhttp.HandlerOpts{Registry: reg}))

	// Parse Pins from Env (libgpiod needs integers)
	var statusPinOffset, togglePinOffset int
	_, err := fmt.Sscanf(statusPinName, "%d", &statusPinOffset)
	if err != nil {
		log.Fatalf("STATUS_PIN must be an integer offset, got: %s", statusPinName)
	}
	_, err = fmt.Sscanf(togglePinName, "%d", &togglePinOffset)
	if err != nil {
		log.Fatalf("TOGGLE_PIN must be an integer offset, got: %s", togglePinName)
	}

	// Initialize GPIO Chip (gpiochip0 is standard on Raspberry Pi)
	chip, err := gpiod.NewChip("gpiochip0")
	if err != nil {
		log.Fatalf("failed to open gpiochip0: %v", err)
	}
	defer chip.Close()

	// Initialize Status Pin as Input with Pull Down
	statusLine, err = chip.RequestLine(statusPinOffset, gpiod.WithPullDown, gpiod.AsInput)
	if err != nil {
		log.Fatalf("failed to request status pin %d: %v", statusPinOffset, err)
	}
	defer statusLine.Close()

	// Initialize Toggle Pin as Output (Initial low)
	toggleLine, err = chip.RequestLine(togglePinOffset, gpiod.AsOutput(0))
	if err != nil {
		log.Fatalf("failed to request toggle pin %d: %v", togglePinOffset, err)
	}
	defer toggleLine.Close()

	// Initialize MQTT client
	opts := mqtt.NewClientOptions().AddBroker(mqttBroker).SetClientID(mqttClientID)
	opts.SetUsername(mqttUsername)
	opts.SetPassword(mqttPassword)
	opts.SetWill(availabilityTopic, "offline", 1, true)
	opts.OnConnect = func(c mqtt.Client) {
		log.Println("Connected to MQTT broker")
		publishDiscoveryMessage(c)
		c.Publish(availabilityTopic, 0, true, "online")
		// Subscribe to the command topic
		if token := c.Subscribe(commandTopic, 0, onCommand); token.Wait() && token.Error() != nil {
			log.Fatalf("failed to subscribe to command topic: %v", token.Error())
		}
	}
	client := mqtt.NewClient(opts)
	if token := client.Connect(); token.Wait() && token.Error() != nil {
		log.Printf("failed to connect to MQTT broker: %v", token.Error())
	}

	// Check initial state
	initialState, _ := statusLine.Value()
	if initialState == 0 {
		promMetrics.onState.Set(0)
	} else {
		promMetrics.onState.Set(1)
	}

	// Goroutine to monitor status pin and publish changes
	go monitorStatusPin(client, statusLine)

	// Keep the application running
	log.Fatal(http.ListenAndServe(metricAddr, nil))
}

func onCommand(client mqtt.Client, msg mqtt.Message) {
	log.Printf("Received command: %s", msg.Payload())
	currentVal, _ := statusLine.Value()

	if string(msg.Payload()) == "ON" && currentVal == 0 {
		togglePower(toggleLine)
	} else if string(msg.Payload()) == "OFF" && currentVal == 1 {
		togglePower(toggleLine)
	}
}

func togglePower(line *gpiod.Line) {
	log.Println("Pulsing toggle pin")
	line.SetValue(1)
	time.Sleep(1 * time.Second)
	line.SetValue(0)
}

func runMountCommand() error {
	if mountCommand == "" {
		return nil
	}
	log.Printf("Running mount command: %s", mountCommand)
	out, err := exec.Command("bash", "-c", mountCommand).CombinedOutput()
	if err != nil {
		return fmt.Errorf("mount command failed: %s, %v", string(out), err)
	}
	log.Printf("Mount command output: %s", string(out))
	return nil
}

func publishDiscoveryMessage(client mqtt.Client) {
	discoveryTopic := fmt.Sprintf("%s/switch/%s/config", haDiscoveryPrefix, switchUniqueID)
	payload := map[string]interface{}{
		"name":               switchName,
		"unique_id":          switchUniqueID,
		"state_topic":        stateTopic,
		"command_topic":      commandTopic,
		"availability_topic": availabilityTopic,
		"payload_on":         "ON",
		"payload_off":        "OFF",
		"state_on":           "ON",
		"state_off":          "OFF",
		"device": map[string]string{
			"identifiers":  "rpi-disk-shelf-controller",
			"name":         "Raspberry Pi Disk Shelf Controller",
			"manufacturer": "Raspberry Pi",
		},
	}
	payloadBytes, _ := json.Marshal(payload)
	token := client.Publish(discoveryTopic, 0, true, payloadBytes)
	token.Wait()
}

func monitorStatusPin(client mqtt.Client, line *gpiod.Line) {
	var lastState int = 0
	for {
		currentState, err := line.Value()
		if err != nil {
			log.Printf("failed to read status pin value: %v", err)
			time.Sleep(1 * time.Second)
			continue
		}

		if currentState == 0 {
			promMetrics.onState.Set(0)
		} else {
			promMetrics.onState.Set(1)
		}

		if currentState != lastState {
			state := "OFF"
			if currentState == 1 {
				state = "ON"
			}
			log.Printf("Disk shelf state changed to: %s", state)
			client.Publish(stateTopic, 0, true, []byte(state))
			lastState = currentState
		}
		time.Sleep(500 * time.Millisecond)
	}
}
