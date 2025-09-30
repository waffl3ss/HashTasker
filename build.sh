#!/bin/bash

# Build script for HashTasker with stripped paths
# This removes local file paths from the compiled binaries

echo "Building HashTasker v1.0.6..."

go mod init hashtasker-go
go mod tidy

# Build server
echo "Building hashtasker-server..."
go build -ldflags="-s -w" -trimpath -o hashtasker-server server.go

# Build worker  
echo "Building hashtasker-worker..."
go build -ldflags="-s -w" -trimpath -o hashtasker-worker worker.go

echo "Build complete!"
echo "Binaries created:"
ls -la hashtasker-server hashtasker-worker 2>/dev/null || echo "Check for any build errors above"