
Output kube-commands.gif

Require echo

Set Shell "zsh"
Set FontSize 16
Set Width 1000
Set Height 600

Type "kubectl get ns" Sleep 500ms  Enter

Sleep 5s

Type "kubectl get pods -n kube-system" Sleep 500ms  Enter

Sleep 5s

Type "kubectl get pods -n system-lb" Sleep 500ms  Enter

Sleep 5s

Type "kubectl get pods -n system-secrets" Sleep 500ms  Enter

Sleep 5s

