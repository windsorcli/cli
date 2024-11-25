
Output demo.gif

Require echo

Set Shell "zsh"
Set FontSize 16
Set Width 1000
Set Height 600

Type "sops edit contexts/local/secrets.enc.yaml" Sleep 500ms  Enter
Sleep 1

Type "dd" Sleep 500ms
Type "dd" Sleep 500ms
Type "dd" Sleep 500ms
Type "dd" Sleep 500ms
Type "dd" Sleep 500ms
Type "iapi_key: plaintext-value" Sleep 500ms  Enter
Type "db_password: plaintext-password" Sleep 500ms  Enter

Sleep 5s

Escape

Type ":wq"  Sleep 500ms  Enter

Sleep 2s

Enter 

Type "windsor env | grep api_key" Sleep 500ms  Enter
Sleep 1s

Type "windsor env | grep db_password" Sleep 500ms  Enter

Sleep 1s

