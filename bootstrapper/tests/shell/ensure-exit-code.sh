#!/bin/bash

# Print some output
echo "This is a test script that explicitly sets an exit code of 0"

# Do a simple check
if [ "$(uname)" != "" ]; then
  echo "Running on: $(uname -a)"
fi

# Test complete - explicitly exit with success
exit 0 
