# Secrets File : 

The sops encrypted file "secrets.enc.yaml" contains env-var-nam/value pairs of secrets.  Windsor generates environment variables based on the contents of this file.  

The secrets yaml file is located in the context's config folder.

**contexts/< context-name >/secrets.enc.yaml**

![secrets](../img/sops-secret.gif)

The windsor env command applies all secrets listed in the context's secrets file.

The secrets file for each context is located here,

$PROJECT_ROOT/contexts/< context-name >/secrets.enc.yaml

The secrets file contains a key/value pairs of secrets that are applied to the shell's environment.


## Try it out

```bash
sops edit contexts/local/secrets.enc.yaml
```

Add these lines

```bash
api_key: plaintext-value
db_password: plaintext-password
```

Save the file.  Confirm environment variables are set.

```bash
env | grep api_key
api_key=plaintext-value

env | grep db_password
db_password=plaintext-password
```
