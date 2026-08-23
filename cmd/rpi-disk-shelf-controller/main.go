package main

import (
	"encoding/json"
	"fmt"
	"log"
	"os"
	"os/exec"
	"os/signal"
	"syscall"
	"time"

	mqtt "github.com/eclipse/paho.mqtt.golang"
	"periph.io/x/conn/v3/gpio"
	"periph.io/x/conn/v3/gpio/gpioreg"
	"periph.io/x/host/v3"
	"periph.io/x/host/v3/gpioioctl" // Linux character device driver
)

var (
	mqttBroker    = os.Getenv("MQTT_BROKER")
	mqttClientID  = os.Getenv("MQTT_CLIENT_ID")
	mqttUsername  = os.Getenv("MQTT_USERNAME")
	mqttPassword  = os.Getenv("MQTT_PASSWORD")
	statusPinName = os.Getenv("STATUS_PIN")
	togglePinName = os.Getenv("TOGGLE_PIN")
	statusPin     gpio.PinIO
	togglePin     gpio.PinIO
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

	// Initialize GPIO
	if _, err := host.Init(); err != nil {
		log.Fatalf("failed to initialize periph: %v", err)
	}

	statusPin = gpiocdev.ByName(statusPinName)
	if statusPin == nil {
		log.Fatalf("failed to find status pin: %s", statusPinName)
	}
	if err := statusPin.In(gpio.PullDown, gpio.BothEdges); err != nil {
		log.Fatalf("failed to set status pin as input: %v", err)
	}

	togglePin = gpiocdev.ByName(togglePinName)
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
		log.Fatalf("failed to connect to MQTT broker: %v", token.Error())
	}

	// Check initial state and turn on if necessary
	initialState := statusPin.Read()
	if initialState == gpio.Low {
		log.Println("Disk shelf is off, turning it on...")
		togglePower(togglePin)
		time.Sleep(5 * time.Second) // Wait for the shelf to power up
	}

	// Run mount command
	if err := runMountCommand(); err != nil {
		log.Printf("failed to run mount command: %v", err)
	}

	// Goroutine to monitor status pin and publish changes
	go monitorStatusPin(client, statusPin)

	// Keep the application running
	c := make(chan os.Signal, 1)
	signal.Notify(c, os.Interrupt, syscall.SIGTERM)
	<-c

	log.Println("Shutting down RPI Disk Shelf Controller")
	client.Publish(availabilityTopic, 0, true, "offline")
	client.Disconnect(250)
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
