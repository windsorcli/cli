### Verify the Signature

Each release includes a checksum file that is signed. To verify the signature, you will need to have GPG installed on your system.

1. **Import the Public Key**: If you haven't already, import the public key used to sign the checksums.
   ```bash
   gpg --keyserver hkp://keyserver.ubuntu.com --recv-keys <public-key-id>
   ```

2. **Verify the Signature**: Download the `.asc` file associated with the checksum file and verify it using:
   ```bash
   gpg --verify <checksum-file>.asc <checksum-file>
   ```

   Replace `<checksum-file>` with the actual checksum file name.

### Check the Checksum

After downloading the binary, you should verify its integrity by checking its checksum.

1. **Download the Checksum File**: Download the checksum file from the release page, which is typically named `checksums.txt`.

2. **Verify the Checksum**: Use the following command to verify the checksum of the downloaded binary:
   - **Linux/macOS**:
     ```bash
     sha256sum -c checksums.txt
     ```
   - **Windows**: Use a tool like `CertUtil` to verify the checksum:
     ```cmd
     CertUtil -hashfile <binary-file> SHA256
     ```

   Ensure that the output indicates that the checksum is correct.

### Example

Here is an example of how you might download and verify a binary for Linux on an `amd64` architecture:

```bash
# Download the binary
wget https://github.com/windsorcli/cli/releases/download/v<version>/windsor-<platform>-<cpu>.tar.gz

# Download the checksum file
wget https://github.com/windsorcli/cli/releases/download/v<version>/windsor-checksums.txt

# Download the signature file
wget https://github.com/windsorcli/cli/releases/download/v<version>/windsor-checksums.txt.sig

# Verify the signature
gpg --verify windsor-checksums.txt.sig windsor-checksums.txt

# Check the checksum
sha256sum -c windsor-checksums.txt
```

Replace `<version>` with the specific version number you are downloading.

### Extract the Binary

1. **Linux/macOS**:
   ```bash
   tar -xzf windsorcli_<version>_linux_amd64.tar.gz
   ```

2. **Windows**:
   Use a tool like WinRAR or 7-Zip to extract the contents of the `windsorcli_<version>_windows_amd64.zip`.

### Move the Binary to Your PATH

1. **Linux/macOS**:
   Move the `windsor` binary to a directory in your `PATH`, such as `/usr/local/bin`:
   ```bash
   sudo mv windsor /usr/local/bin/
   ```

2. **Windows**:
   Move the `windsor.exe` to a directory included in your `PATH`, such as `C:\Program Files\WindsorCLI`. You may need to add this directory to your `PATH` environment variable if it's not already included.

