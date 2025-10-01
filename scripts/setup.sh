#!/bin/bash
# deploy_ec2.sh - Script to setup and run the Docker checkpoint project on EC2

set -e

echo "=== EC2 Docker Checkpoint Setup Script ==="
echo

# Function to check if a command exists
command_exists() {
    command -v "$1" >/dev/null 2>&1
}

# Install Docker if not present
install_docker() {
    if ! command_exists docker; then
        echo "Installing Docker..."
        sudo apt-get update
        sudo apt-get install -y \
            ca-certificates \
            curl \
            gnupg \
            lsb-release

        curl -fsSL https://download.docker.com/linux/ubuntu/gpg | sudo gpg --dearmor -o /usr/share/keyrings/docker-archive-keyring.gpg

        echo \
          "deb [arch=$(dpkg --print-architecture) signed-by=/usr/share/keyrings/docker-archive-keyring.gpg] https://download.docker.com/linux/ubuntu \
          $(lsb_release -cs) stable" | sudo tee /etc/apt/sources.list.d/docker.list > /dev/null

        sudo apt-get update
        sudo apt-get install -y docker-ce docker-ce-cli containerd.io
        sudo usermod -aG docker $USER
        echo "Docker installed successfully!"
        echo "Note: Please logout and login again for docker group changes to take effect"
    else
        echo "Docker is already installed"
        echo "Ensuring user is in docker group..."
        sudo usermod -aG docker $USER
        echo "Note: Please logout and login again for docker group changes to take effect"
    fi
}

# Install CRIU
install_criu() {
    if ! command_exists criu; then
        echo "Installing CRIU..."
        sudo apt-get update
        sudo apt-get install -y criu

        # Set capabilities for CRIU
        sudo setcap cap_sys_admin,cap_sys_ptrace,cap_sys_chroot+ep $(which criu)

        # Verify installation
        criu --version
        echo "CRIU installed successfully!"
    else
        echo "CRIU is already installed"
        criu --version
    fi
}

# Install Go if not present
install_go() {
    if ! command_exists go; then
        echo "Installing Go..."
        GO_VERSION="1.21.5"
        wget -q "https://go.dev/dl/go${GO_VERSION}.linux-amd64.tar.gz"
        sudo rm -rf /usr/local/go
        sudo tar -C /usr/local -xzf "go${GO_VERSION}.linux-amd64.tar.gz"
        rm "go${GO_VERSION}.linux-amd64.tar.gz"

        # Add Go to PATH
        export PATH=$PATH:/usr/local/go/bin
        echo 'export PATH=$PATH:/usr/local/go/bin' >> ~/.bashrc
        export PATH=$PATH:/usr/local/go/bin

        go version
        echo "Go installed successfully!"
    else
        echo "Go is already installed"
        go version
    fi
}

# Enable Docker experimental features for checkpoint support
enable_docker_experimental() {
    echo "Enabling Docker experimental features..."
    sudo mkdir -p /etc/docker

    # Create or update Docker daemon configuration
    sudo tee /etc/docker/daemon.json > /dev/null <<EOF
{
    "experimental": true,
    "live-restore": true
}
EOF

    # Restart Docker to apply changes
    sudo systemctl restart docker
    echo "Docker experimental features enabled!"
}

# Build the checkpoint application
build_application() {
    echo "Building the Docker checkpoint application..."
    cd /home/ubuntu/lab3-docker-container-restoring-with-GO-criu


    # Download dependencies and create go.sum
    go mod tidy
    go mod download

    # Build the application
    go build -o docker-checkpoint

    # Make it executable
    chmod +x docker-checkpoint
    

    echo "Application built successfully!"
}



# Main execution
main() {
    echo "Starting EC2 setup for Docker checkpoint project..."
    echo "This script will install Docker, CRIU, and Go if they're not present"
    echo

    # Update system packages
    echo "Updating system packages..."
    sudo apt-get update

    # Install prerequisites
    sudo apt-get install -y wget curl git build-essential

    # Install required software
    install_docker
    install_criu
    install_go
    enable_docker_experimental

    # Build the application
    echo
    echo "Building the checkpoint application..."
    build_application

    echo
    echo "=== Setup Complete ==="
    echo
    echo "IMPORTANT: Please logout and login again (or run 'newgrp docker') to activate Docker group membership"
    echo
    echo "After re-login, you can use the checkpoint tool:"
    echo "  sudo ./docker-checkpoint -container <container-name>"
    echo
    echo "To run the test script:"
    echo "  sudo ./test_checkpoint.sh"
    echo
    echo "Example usage:"
    echo "  1. Start a container: docker run -d --name myapp alpine sleep 3600"
    echo "  2. Checkpoint it: sudo ./docker-checkpoint -container myapp"
    echo "  3. Restore it: sudo ./docker-checkpoint -restore -container myapp -new-name myapp-restored"
    echo

   
}

# Run main function
main
