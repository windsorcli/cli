# Guidelines for Writing Tests
## Objective
Enhance the readability, organization, and maintainability of test files by following these guidelines.

### 1. Extract Helper Functions
- **Identify Repeated Code**: Look for code that is repeated across multiple test cases.
- **Create Reusable Helpers**: Extract common setup, teardown, and operations into reusable helper functions.
- **Use TestMain for Global Setup/Teardown**: Implement TestMain to execute setup() and teardown() before and after tests when necessary.

**Example:**
```go
Copy code
func setup() {
    // Initialization code
}

func teardown() {
    // Cleanup code
}

func TestMain(m *testing.M) {
    setup()
    code := m.Run()
    teardown()
    os.Exit(code)
}
```

### 2. Organize Tests with Subtests
- **Group Related Tests**: Use t.Run() to group related test cases into subtests.
- **Descriptive Subtest Names**: Give each subtest a descriptive name indicating its purpose.
- **Focus on One Function per Test**: Create one func Test... per function being tested, using subtests for different scenarios.

**Example:**
```go
func TestMyFunction(t *testing.T) {
    t.Run("ValidInput", func(t *testing.T) {
        // Test code for valid input
    })

    t.Run("InvalidInput", func(t *testing.T) {
        // Test code for invalid input
    })
}
```

### 3. Add BDD-Style Comments
- **Descriptive Comments**: Use Behavior-Driven Development (BDD) style comments to clarify test scenarios.
- **Use Given, When, Then**: Structure comments with "Given", "When", and "Then" to outline context, action, and expected outcome.

**Example:**
```go
    t.Run("ValidUserCredentials", func(t *testing.T) {
    // Given: a user with valid credentials
    // When: the user attempts to log in
    // Then: the login should be successful
})
```

### 4. Use t.Cleanup for Deferred Cleanup
Ensure Proper Cleanup: Register cleanup functions with t.Cleanup() to run after each test completes.
- **Resource Management**: Guarantees that resources are released even if a test fails.

**Example:**
```go
func TestDatabaseOperation(t *testing.T) {
    db := setupDatabase()
    t.Cleanup(func() {
        db.Close()
    })

    // Test code using db
}
```

### 5. Ensure Consistent Naming
- **Clear Naming Conventions**: Follow consistent naming for functions, variables, and tests.
- **Descriptive Names**: Use names that clearly describe the purpose or action.
6. Improve Error Handling
- **Clear Error Messages**: Provide error messages that offer context and aid in debugging.
- **Understandable Failures**: Ensure that test failures are easy to understand and trace back to the cause.

**Example:**
```go
if err != nil {
    t.Fatalf("Expected no error, got %v", err)
}
```

### 7. Maintain Existing Functionality
- **No Functional Changes**: Focus on improving test code without altering the functionality being tested.
- **Refactor Carefully**: Ensure that any changes do not affect the test outcomes.

### 8. Use Existing Mocks
- **Leverage Ready-Made Mocks**: Utilize mocks already present in the codebase.
- **Avoid Unnecessary Mocks**: Do not introduce new mocks unless absolutely necessary.

### 9. Remove Redundant Tests
- **Eliminate Duplication**: Identify and remove tests that duplicate existing coverage.
- **Streamline Test Suite**: Keep the test suite concise and focused on unique scenarios.

By adhering to these guidelines, you can write tests that are clean, organized, and easy to maintain, ultimately contributing to a more robust and reliable codebase.

### Short Style Examples
Example 1: Extract Helper Functions
Extract common setup and teardown logic into helper functions to avoid repetition.

```go
func setup() {
    // Initialization code
}

func teardown() {
    // Cleanup code
}

func TestMain(m *testing.M) {
    setup()
    code := m.Run()
    teardown()
    os.Exit(code)
}
```
Example 2: Organize Tests with t.Run
Use t.Run() to create subtests, improving organization and readability.

```go
func TestCalculate(t *testing.T) {
    t.Run("PositiveNumbers", func(t *testing.T) {
        // Test with positive numbers
    })

    t.Run("NegativeNumbers", func(t *testing.T) {
        // Test with negative numbers
    })
}
```
Example 3: Add BDD-Style Comments
Incorporate BDD-style comments to outline test scenarios clearly.

```go
t.Run("UserExists", func(t *testing.T) {
    // Given: a user exists in the database
    // When: we fetch the user by ID
    // Then: we should receive the correct user data
})
```
Example 4: Use t.Cleanup for Deferred Cleanup
Ensure resources are properly released after each test using t.Cleanup().

```go
func TestFileOperation(t *testing.T) {
    file, err := os.Create("temp.txt")
    if err != nil {
        t.Fatal(err)
    }
    t.Cleanup(func() {
        os.Remove("temp.txt")
    })

    // Test code using file
}
```
