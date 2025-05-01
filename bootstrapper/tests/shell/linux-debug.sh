#!/bin/bash

# This script is specifically for debugging Linux test issues

echo "==== Linux Debug Test Script ===="
echo "Date: $(date)"
echo "Kernel: $(uname -a)"
echo "Current directory: $(pwd)"
echo "GITHUB_WORKSPACE: $GITHUB_WORKSPACE"
echo "PATH: $PATH"
echo "Shell: $SHELL"
echo "User: $(whoami)"

echo "==== Process Info ===="
ps aux | head -10

echo "==== Directory Structure ===="
ls -la

# Test for common exit code issues
echo "==== Bash Version ===="
bash --version

echo "==== Last Command Status Handling Test ===="
# This line sets the status to 1
false
# But we explicitly reset it to 0 for the script exit
exit 0 
