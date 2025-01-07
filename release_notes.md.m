     To verify the signature and checksum of the binary:
     1. Download the checksum file and the signature file.
     2. Use `gpg --verify <signature-file> <checksum-file>` to verify the signature.
     3. Use `sha256sum -c <checksum-file>` to verify the checksum.