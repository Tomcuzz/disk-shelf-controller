# RPI Disk Shelf Controller

This project provides a Go-based application that runs in a Docker container to control a disk shelf connected to a Raspberry Pi's GPIO pins. It is designed to be deployed in a Kubernetes cluster and integrates with Home Assistant via MQTT for easy control and monitoring.

## Features

- **GPIO Control**: Manages a disk shelf using two GPIO pins—one for status monitoring and one for power toggling.
- **Auto Power-On**: Ensures the disk shelf is powered on when the controller starts.
- **Custom Mount Command**: Executes a configurable shell command to mount the disk shelf's storage.
- **Home Assistant Integration**: Exposes the disk shelf as a switch in Home Assistant using MQTT discovery.
- **Kubernetes Ready**: Includes a sample Kubernetes deployment configuration.
- **Containerized**: Packaged as a lightweight, multi-stage Docker image optimized for ARM architecture.

## Configuration

The controller is configured using environment variables, which can be set in the `deployment.yaml` file or passed directly to the Docker container.

| Environment Variable | Description                                     | Default Value |
| -------------------- | ----------------------------------------------- | ------------- |
| `MQTT_BROKER`        | URL of your MQTT broker                         | (required)    |
| `MQTT_CLIENT_ID`     | Client ID for the MQTT connection               | (required)    |
| `MQTT_USERNAME`      | Username for MQTT authentication                | (optional)    |
| `MQTT_PASSWORD`      | Password for MQTT authentication                | (optional)    |
| `STATUS_PIN`         | GPIO pin name for disk shelf status (e.g., `GPIO23`) | (required)    |
| `TOGGLE_PIN`         | GPIO pin name for power toggling (e.g., `GPIO24`)  | (required)    |
| `MOUNT_COMMAND`      | Shell command to mount the disk shelf's storage | (optional)    |

## Getting Started

### Prerequisites

- A Raspberry Pi with GPIO pins connected to your disk shelf.
- Docker installed on your development machine.
- A Kubernetes cluster with a Raspberry Pi node.
- An MQTT broker (e.g., Mosquitto) for Home Assistant integration.

### Build and Run with Docker

1.  **Clone the repository**:
    ```sh
    git clone <repository-url>
    cd rpi-disk-shelf-controller
    ```

2.  **Build the Docker image**:
    ```sh
    docker build -t your-repo/rpi-disk-shelf-controller:latest .
    ```

3.  **Push the image to a container registry**:
    ```sh
    docker push your-repo/rpi-disk-shelf-controller:latest
    ```

### Deploy to Kubernetes

1.  **Configure the deployment**:
    -   Open the `deployment.yaml` file.
    -   Update the `image` field to point to the image you pushed.
    -   Set the environment variables with your specific configuration.
    -   Adjust the `nodeSelector` to match the hostname of your Raspberry Pi node.

2.  **Apply the deployment**:
    ```sh
    kubectl apply -f deployment.yaml
    ```

## Home Assistant Integration

The controller uses MQTT discovery to automatically add a switch entity to Home Assistant.

-   **Discovery Topic**: `homeassistant/switch/disk_shelf_switch/config`
-   **State Topic**: `rpi-disk-shelf-controller/switch/state`
-   **Command Topic**: `rpi-disk-shelf-controller/switch/command`
-   **Availability Topic**: `rpi-disk-shelf-controller/status`

Once the controller is running, a new switch named "Disk Shelf" will appear in your Home Assistant dashboard, allowing you to turn the disk shelf on or off and monitor its status.
