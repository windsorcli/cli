Follow these steps to install Windsor CLI from the source code:

#### Step 1: Clone the Repository
```bash
git clone https://github.com/windsor-hotel/cli.git
```

#### Step 2: Build the Application

```bash
cd cli;mkdir -p dist;go build -o dist/windsor cmd/windsor/main.go;cd ..
```

#### Step 3: Put application in system PATH

```bash
cp cli/dist/windsor /usr/local/bin/windsor
```

<div>
{{ previous_footer('Installation', '../index.html') }}
</div>

<script>
  document.getElementById('previousButton').addEventListener('click', function() {
    window.location.href = '../index.html'; 
  });
</script>