TEST CHANGE

# [WindsorCLI](https://windsor-hotel.github.io/windsorcli/)
The Windsor Command Line Interface (CLI) is a powerful tool designed to streamline your workflow and enhance productivity. With a suite of intuitive commands, the Windsor CLI allows you to efficiently manage your projects, automate repetitive tasks, and integrate seamlessly with other tools in your development environment.

## [Bootstrapping Local Cluster](#bootstrapping-local-cluster)

## ![bootstrap](./img/k9s-pods.gif)

<!-- <div class="vertical-scrolling-images">
  <img src="img/icon.svg" alt="Feature 1">
  <img src="img/icon.svg" alt="Feature 2">
  <img src="img/icon.svg" alt="Feature 3">
</div> -->

## [Purpose](#purpose)

The Windsor CLI is designed to simplify and enhance the development workflow for developers working on various projects. Its primary purpose is to provide a unified command-line interface that streamlines project setup, configuration management, and integration with other tools. By offering a consistent and efficient way to manage projects, the Windsor CLI aims to reduce the time and effort required for repetitive tasks, allowing developers to focus more on coding and less on setup and configuration.

Key objectives of the Windsor CLI include:

- **Efficiency**: Automate routine tasks to save time and reduce manual errors.
- **Consistency**: Ensure a standardized approach to project management across different environments and teams.
- **Flexibility**: Support a wide range of configurations and integrations to accommodate diverse project needs.
- **Cross-Platform Compatibility**: Provide a seamless experience on Windows, macOS, and Linux systems.

By achieving these objectives, the Windsor CLI empowers developers to work more productively and collaboratively, ultimately leading to faster and more reliable project delivery.

## [Features](#features)
- **Project Initialization**: Quickly set up new projects with predefined configurations.
- **Configuration Management**: Easily manage and switch between different project configurations.
- **Shell Integration**: Seamlessly integrates with your shell environment for enhanced productivity.
- **Cross-Platform Support**: Works on Windows, macOS, and Linux.


This will ensure that the Windsor CLI environment variables are loaded every time a new shell session is started.

## [Contributing](#contributing)
We welcome contributions to Windsor CLI! If you would like to contribute, please follow these steps:

1. Fork the repository.
2. Create a new branch for your feature or bugfix.
3. Make your changes and commit them with a descriptive message.
4. Push your changes to your fork.
5. Create a pull request to the main repository.

Please ensure that your code adheres to our coding standards and includes appropriate tests.

## [License](#license)

Windsor CLI is licensed under the Mozilla Public License Version 2.0. See the [LICENSE](LICENSE) file for more details.


## [Contact Information](#contact-information)

Thank you for using Windsor CLI! If you have any questions or need further assistance, please feel free to open an issue on our GitHub repository.


<!-- Add buttons to load new files -->
<button id="quickStartButton">Quick Start</button>
<button id="demoButton">Local Cluster Demo</button>

<script>
  document.getElementById('quickStartButton').addEventListener('click', function() {
    window.location.href = 'tutorial/quick-start/index.html'; 
  });

  document.getElementById('demoButton').addEventListener('click', function() {
    window.location.href = 'tutorial/local-cluster-demo/index.html'; 
  });
</script>

<div>
{{ next_footer('Installation', 'install/index.html') }}
</div>

<script>
  document.getElementById('nextButton').addEventListener('click', function() {
    window.location.href = 'install/index.html'; 
  });
</script>
