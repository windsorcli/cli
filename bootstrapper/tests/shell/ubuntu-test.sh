#!/bin/bash

# Simple test script to understand Ubuntu behavior

# Enable debugging
set -x

# Test if we're running on Linux
OS=$(uname -s)
echo "Running on: $OS"

# Test for common issues
echo "Testing error handling..."

# This is the simplest script that should work
echo "Success!"

# Explicitly exit with success
exit 0 
