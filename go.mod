module github.com/tomcuzz/rpi-disk-shelf-controller

go 1.25.0

require (
	github.com/eclipse/paho.mqtt.golang v1.3.5
	github.com/prometheus/client_golang v1.24.1
	github.com/warthog618/gpiod v0.8.2
	periph.io/x/conn/v3 v3.7.0
	periph.io/x/host/v3 v3.7.0
)

require (
	github.com/beorn7/perks v1.0.1 // indirect
	github.com/cespare/xxhash/v2 v2.3.0 // indirect
	github.com/gorilla/websocket v1.4.2 // indirect
	github.com/munnerz/goautoneg v0.0.0-20191010083416-a7dc8b61c822 // indirect
	github.com/prometheus/client_model v0.6.2 // indirect
	github.com/prometheus/common v0.70.1 // indirect
	github.com/prometheus/procfs v0.21.1 // indirect
	golang.org/x/net v0.57.0 // indirect
	golang.org/x/sys v0.47.0 // indirect
	google.golang.org/protobuf v1.36.11 // indirect
	periph.io/x/d2xx v0.0.1 // indirect
)

replace periph.io/x/d2xx => github.com/periph/d2xx v0.0.1
