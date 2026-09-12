package main

import (
	"encoding/json"
	"fmt"
	"log"
	"os"
	"os/exec"
	"time"

	"net/http"

	mqtt "github.com/eclipse/paho.mqtt.golang"
	"periph.io/x/conn/v3/gpio"
	"periph.io/x/conn/v3/gpio/gpioreg"
	"periph.io/x/host/v3"

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
	statusPinName = os.Getenv("STATUS_PIN")
	togglePinName = os.Getenv("TOGGLE_PIN")
	mountCommand  = os.Getenv("MOUNT_COMMAND")
	statusPin     gpio.PinIO
	togglePin     gpio.PinIO
	metricAddr	  = os.Getenv("listen-address")
	promMetrics	  *metrics
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

	// Setup Metrics
	log.Println("Settng up metrics")
	if len(metricAddr) == 0 {
        metricAddr = ":8080"
    }
	reg := prometheus.NewRegistry()
	promMetrics = &metrics{
		onState: promauto.With(reg).NewGauge(prometheus.GaugeOpts{
			Name: "disk-shelf_on-state",
			Help: "The current on/off state of the disk shelf",
		}),
	}
	reg.MustRegister(
		collectors.NewGoCollector(),
		collectors.NewProcessCollector(collectors.ProcessCollectorOpts{}),
	)
	reg.MustRegister(promMetrics.onState)
	http.Handle("/metrics", promhttp.HandlerFor(reg, promhttp.HandlerOpts{Registry: reg}))

	// Initialize GPIO
	if _, err := host.Init(); err != nil {
		log.Fatalf("failed to initialize periph: %v", err)
	}

	statusPin = gpioreg.ByName(statusPinName)
	if statusPin == nil {
		log.Fatalf("failed to find status pin: %s", statusPinName)
	}
	if err := statusPin.In(gpio.PullDown, gpio.BothEdges); err != nil {
		log.Fatalf("failed to set status pin as input: %v", err)
	}

	togglePin = gpioreg.ByName(togglePinName)
	if togglePin == nil {
		log.Fatalf("failed to find toggle pin: %s", togglePinName)
	}
	if err := togglePin.Out(gpio.Low); err != nil {
		log.Fatalf("failed to set toggle pin as output: %v", err)
	}

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
		log.Warningf("failed to connect to MQTT broker: %v", token.Error())
	}

	// Check initial state and turn on if necessary
	initialState := statusPin.Read()
	if initialState == gpio.Low {
		promMetrics.onState.Set(0)
		// log.Println("Disk shelf is off, turning it on...")
		// togglePower(togglePin)
		// time.Sleep(5 * time.Second) // Wait for the shelf to power up
	} else {
		promMetrics.onState.Set(1)
	}

	// Run mount command
	// if err := runMountCommand(); err != nil {
	// 	log.Printf("failed to run mount command: %v", err)
	// }

	// Goroutine to monitor status pin and publish changes
	go monitorStatusPin(client, statusPin)

	// Keep the application running
	log.Fatal(http.ListenAndServe(metricAddr, nil))
}

func onCommand(client mqtt.Client, msg mqtt.Message) {
	log.Printf("Received command: %s", msg.Payload())
	if string(msg.Payload()) == "ON" && statusPin.Read() == gpio.Low {
		togglePower(togglePin)
	} else if string(msg.Payload()) == "OFF" && statusPin.Read() == gpio.High {
		togglePower(togglePin)
	}
}

func togglePower(pin gpio.PinIO) {
	log.Println("Pulsing toggle pin")
	pin.Out(gpio.High)
	time.Sleep(1 * time.Second)
	pin.Out(gpio.Low)
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

func monitorStatusPin(client mqtt.Client, pin gpio.PinIO) {
	var lastState gpio.Level = gpio.Low
	for {
		currentState := pin.Read()
		if currentState == gpio.Low {
			promMetrics.onState.Set(0)
		} else {
			promMetrics.onState.Set(1)
		}
		if currentState != lastState {
			state := "OFF"
			if currentState == gpio.High {
				state = "ON"
			}
			log.Printf("Disk shelf state changed to: %s", state)
			client.Publish(stateTopic, 0, true, []byte(state))
			lastState = currentState
		}
		time.Sleep(500 * time.Millisecond)
	}
}
