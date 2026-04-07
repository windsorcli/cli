---
title: "Trusted Folders"
description: "The Windsor CLI will only run in folders you trust."
---
# Trusted Folders
The Windsor CLI performs certain actions based on the contents of project files. You most often pull these files from a repository that you or another party manages. This is a potential vector for environment injection attacks. You should always familiarize yourself with a project's Windsor configuration and trust the author of the project.

To provide additional protection, Windsor will not inject [Windsor environment](../guides/environment-injection.md) values unless you have executed `windsor init` in the project folder. This acknowledges your intention to actively develop within this project. To track this, the Windsor CLI maintains a list of trusted repository folders in a `$HOME/.config/windsor/.trusted` folder. Any folder or subfolder of one listed here, is susceptible to environment execution by Windsor.

<div>
  {{ footer('Hello, World!', '../../tutorial/hello-world/index.html', 'Securing Secrets', '../secrets/index.html') }}
</div>

<script>
  document.getElementById('previousButton').addEventListener('click', function() {
    window.location.href = '../../tutorial/hello-world/index.html'; 
  });
  document.getElementById('nextButton').addEventListener('click', function() {
    window.location.href = '../secrets/index.html'; 
  });
</script>
