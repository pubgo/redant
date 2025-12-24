# Args Parsing Test Example

This example demonstrates various parameter formats and parsing scenarios supported by the redant framework.

## Supported Parameter Formats

### 1. Multiple Positional Arguments

Directly pass multiple parameter values:

```bash
./args-test multi arg1 arg2 arg3
```

Output:
```
=== Multiple Positional Arguments ===
Args count: 3
  arg[0]: arg1
  arg[1]: arg2
  arg[2]: arg3
```

### 2. URL Query Format

Using `&` separated key-value pairs, similar to URL query strings:

```bash
./args-test query "name=John&age=30&tags=go&tags=cli"
```

Output:
```
=== URL Query String Format ===
Args: [name=John&age=30&tags=go&tags=cli]
Parsed query parameters:
  name: John
  age: 30
  tags: [go cli]
```

**Features:**
- Use `&` to separate multiple key-value pairs
- Support duplicate key names (like `tags=go&tags=cli`)
- **Parameters are passed as args, need to parse using `ParseQueryArgs()` in handler**

### 3. Form Data Format

Using space-separated key-value pairs, similar to HTML form data:

```bash
./args-test form "user=admin email=admin@example.com active=true"
```

Output:
```
=== Form Data Format ===
Args: [user=admin email=admin@example.com active=true]
Parsed form parameters:
  user: admin
  email: admin@example.com
  active: true
```

**Features:**
- Use space to separate multiple key-value pairs
- Support quoted values (single or double quotes)
- **Parameters are passed as args, need to parse using `ParseFormArgs()` in handler**

### 4. JSON Format

Support JSON object and array formats:

#### JSON Object Format

```bash
./args-test json '{"id":123,"title":"Test","count":42}'
```

Output:
```
=== JSON Format ===
Args: [{"id":123,"title":"Test","count":42}]
Parsed JSON parameters:
  id: 123
  title: Test
  count: 42
```

#### JSON Array Format

```bash
./args-test json '["value1","value2","value3"]'
```

Output:
```
=== JSON Format ===
Args: [["value1","value2","value3"]]
Parsed JSON parameters:
  [array]: [value1 value2 value3]
```

**Features:**
- Support JSON object format `{...}`
- Support JSON array format `[...]`
- **Parameters are passed as args, need to parse using `ParseJSONArgs()` in handler**
- JSON objects are parsed into key-value mappings
- JSON arrays are parsed into positional parameter lists

### 5. Mixed Format

Can mix positional parameters and key-value pair formats:

```bash
./args-test mixed "positional1" "name=test"
```

Output:
```
=== Mixed Format ===
Args: [positional1 name=test]
Positional arg[0]: positional1
Query arg[1]: map[name:[test]]
```

### 6. Complex Scenarios

Mix multiple formats:

```bash
./args-test complex "pos1" "pos2" "flag1=value1" "flag2=100"
```

Output:
```
=== Complex Scenario ===
Args: [pos1 pos2 flag1=value1 flag2=100]
Positional arg[0]: pos1
Positional arg[1]: pos2
Query arg[2]: map[flag1:[value1]]
Query arg[3]: map[flag2:[100]]
```

Using JSON format:

```bash
./args-test complex "pos1" '{"flag1":"json-value","flag2":200}'
```

Output:
```
=== Complex Scenario ===
Args: [pos1 {"flag1":"json-value","flag2":200}]
Positional arg[0]: pos1
JSON arg[1]: map[flag1:[json-value] flag2:[200]]
```

## Handling Conflicts with Subcommands

### Scenario 1: Parameter Format vs Subcommand

When parameters look like subcommands, the system prioritizes matching subcommands:

```bash
# Execute parent command (parameter format)
./args-test conflict "value=test"
```

Output:
```
=== Conflict Parent Command ===
Args: [value=test]
Parsed query parameters:
  value: [test]
```

```bash
# Execute subcommand
./args-test conflict sub
```

Output:
```
=== Conflict Subcommand ===
Args: []
```

```bash
# Subcommand with parameters
./args-test conflict sub arg1 arg2
```

Output:
```
=== Conflict Subcommand ===
Args: [arg1 arg2]
```

### Parsing Rules

1. **Command Parsing Priority:**
   - First try exact matching of subcommand names
   - If subcommand is matched, execute the subcommand
   - If no match, continue parsing parameters

2. **Parameter Recognition Rules:**
   - Starts with `-`: recognized as flag
   - Contains `=` and does not start with `-`: recognized as key-value pair parameter
   - Starts with `{` or `[`: recognized as JSON format
   - Others: recognized as positional parameter

3. **Parameter Format Detection:**
   - Contains `&` or no spaces: URL Query format
   - Contains spaces: Form Data format
   - Starts with `{` or `[`: JSON format

## Running All Tests

Use the provided test script to run all test cases:

```bash
./test.sh
```

## Notes

1. **Quote Usage:** In shell, parameters containing special characters need to be wrapped in quotes
2. **JSON Format:** JSON strings need to be wrapped in single quotes to avoid shell parsing
3. **Parameter Order:** Positional and key-value parameters can be mixed
4. **Subcommand Priority:** Subcommand names take precedence over parameter parsing
5. **Parameter Parsing:** Query, Form and JSON format parameters are all passed as `args`, and need to be parsed using corresponding functions in the handler:
   - `redant.ParseQueryArgs()` - Parse URL query format
   - `redant.ParseFormArgs()` - Parse Form data format
   - `redant.ParseJSONArgs()` - Parse JSON format

## Test Case Summary

| Test Case | Command Example | Description |
|---------|---------|------|
| Multiple Positional Args | `multi arg1 arg2 arg3` | Basic positional parameters |
| URL Query | `query "name=John&age=30"` | URL query string format |
| Form Data | `form "user=admin email=test"` | Form data format |
| JSON Object | `json '{"id":123}'` | JSON object format |
| JSON Array | `json '["v1","v2"]'` | JSON array format |
| Mixed Format | `mixed pos1 "name=test"` | Positional parameters + key-value pairs |
| Subcommand Conflict | `conflict sub` | Subcommand priority |
| Complex Scenario | `complex pos1 "flag1=v1"` | Mixed multiple formats |