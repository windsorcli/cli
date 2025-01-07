
# Steps

- Create gpg key
- Add secrets to the repository
- 

# Requirements

- secrets.GPG_PRIVATE_KEY # See creating gpg below
- secrets.PASSPHRASE
- secrets.HOMEBREW_ACCESS

# Creating a GPG Private Key and Fingerprint

## Step 1: Install GPG

If you don't have GPG installed, you can install it using the following commands:

- **On macOS:** Use Homebrew
  ```bash
  brew install gnupg
  ```

- **On Linux:** Use your package manager
  ```bash
  sudo apt-get install gnupg
  ```

- **On Windows:** Download and install from [GnuPG's official site](https://gnupg.org/download/).

## Step 2: Generate a GPG Key

1. **Open a Terminal:**
   - On Windows, you can use Git Bash or any terminal emulator.

2. **Generate the Key:**
   ```bash
   gpg --full-generate-key
   ```

3. **Follow the Prompts:**
   - **Key Type:** Choose the default (RSA and RSA).
   - **Key Size:** 4096 bits is recommended for strong security.
   - **Key Expiration:** Choose a suitable expiration period or select "0" for no expiration.
   - **User ID Information:** Enter your name, email, and an optional comment.
   - **Passphrase:** Choose a strong passphrase to protect your key.

## Step 3: Retrieve the GPG Fingerprint

After generating the key, you can list your keys and find the fingerprint:

```bash
gpg --list-keys
```

This will display a list of keys. Look for the key you just created, and you will see a line that starts with `pub` followed by the key ID and the fingerprint.

## Step 4: Export the GPG Private Key

To export your private key, use the following command:

```bash
gpg --armor --export-secret-keys your-email@example.com > private-key.asc
```

Replace `your-email@example.com` with the email address you used when creating the key. This will create a file named `private-key.asc` containing your private key in ASCII format.

## Step 5: Use the Key and Fingerprint in GitHub Actions

- **GPG Private Key:** Open `private-key.asc` in a text editor and copy its contents. Add this as the `GPG_PRIVATE_KEY` secret in your GitHub repository.
- **GPG Fingerprint:** Copy the fingerprint from the `gpg --list-keys` output and add it as the `GPG_FINGERPRINT` secret in your GitHub repository.

By following these steps, you'll have a GPG private key and fingerprint ready to use in your GitHub Actions workflows.

# Brew

## Add brew section to the .goreleaser.yaml

## Create new public repo

Name: homebrew-<repo>

```
brew tap tvangundy/cli
brew install windsor
```

# How to verify binary with signature and checksums
https://developer.hashicorp.com/well-architected-framework/operational-excellence/verify-hashicorp-binary

# User Instructions 

     To verify the signature and checksum of the binary:
     1. Download the checksum file and the signature file.
     2. Use `gpg --verify <signature-file> <checksum-file>` to verify the signature.
     3. Use `sha256sum -c <checksum-file>` to verify the checksum.
