#!/bin/bash

APP_NAME="im2msch"

echo "Building for Linux..."
GOOS=linux GOARCH=amd64 go build -o ./build/${APP_NAME}_linux

echo "Building for Windows..."
GOOS=windows GOARCH=amd64 go build -o ./build/${APP_NAME}_windows.exe

echo "Done!"