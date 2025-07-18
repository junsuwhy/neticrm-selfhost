# netiCRM Self-Host Installer Compilation Guide

This document explains how to compile the netiCRM Self-Host installer.

## Prerequisites

- Install [Go programming language](https://golang.org/dl/) (version 1.18 or newer recommended)
- Make sure `GOPATH` environment variable is properly set
- Git version control system installed on your system

## Installing Dependencies

Before compiling, you need to install the required dependencies:

```bash
go get github.com/AlecAivazis/survey/v2
go get github.com/fatih/color
go get github.com/joho/godotenv
```

If you're using Go 1.18+ version, you can also use:

```bash
go get -u github.com/AlecAivazis/survey/v2@latest
go get -u github.com/fatih/color@latest
go get -u github.com/joho/godotenv@latest
```

## Compilation Commands

To compile the installer, execute the following command from the project root directory:

```bash
# Build the program directly from the project root
go build -o install ./cmd/install
```

For cross-platform compilation, you can set the GOOS and GOARCH environment variables:

```bash
# For Windows
GOOS=windows GOARCH=amd64 go build -o install.exe ./cmd/install

# For Linux
GOOS=linux GOARCH=amd64 go build -o install ./cmd/install

# For macOS
GOOS=darwin GOARCH=amd64 go build -o install ./cmd/install
```

## Running the Compiled Program

After compilation, you can run the installer from the netiCRM Self-Host directory:

```bash
# From the project root directory
./install

# Run with verbose output for more detailed system messages
./install -v
```

## Important Notes

- Ensure that the `example.env` file is copied and configured correctly before running the installer
- The program requires Docker and Docker Compose environment to run properly
- Administrative privileges may be required to manage Docker containers during installation
