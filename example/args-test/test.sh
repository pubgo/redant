#!/bin/bash

# Args test script
# Test various argument formats and subcommand conflict scenarios

APP="./args-test"

echo "=========================================="
echo "Args Parsing Test"
echo "=========================================="
echo ""

# Test 1: Multiple regular arguments
echo "Test 1: Multiple regular arguments"
echo "Command: $APP multi arg1 arg2 arg3"
echo "---"
$APP multi arg1 arg2 arg3
echo ""
echo ""

# Test 2: URL Query format
echo "Test 2: URL Query format"
echo "Command: $APP query \"name=John&age=30&tags=go&tags=cli\""
echo "---"
$APP query "name=John&age=30&tags=go&tags=cli"
echo ""
echo ""

# Test 3: Form Data format
echo "Test 3: Form Data format"
echo "Command: $APP form \"user=admin email=admin@example.com active=true\""
echo "---"
$APP form "user=admin email=admin@example.com active=true"
echo ""
echo ""

# Test 4: JSON format (object)
echo "Test 4: JSON format (object)"
echo "Command: $APP json '{\"id\":123,\"title\":\"Test\",\"count\":42}'"
echo "---"
$APP json '{"id":123,"title":"Test","count":42}'
echo ""
echo ""

# Test 5: JSON format (array)
echo "Test 5: JSON format (array)"
echo "Command: $APP json '[\"value1\",\"value2\",\"value3\"]'"
echo "---"
$APP json '["value1","value2","value3"]'
echo ""
echo ""

# Test 6: Mixed format - positional + Query
echo "Test 6: Mixed format - positional + Query"
echo "Command: $APP mixed positional1 name=test"
echo "---"
$APP mixed positional1 name=test
echo ""
echo ""

# Test 7: Conflict with subcommand - should execute parent command
echo "Test 7: Conflict with subcommand - execute parent command"
echo "Command: $APP conflict value=test"
echo "---"
$APP conflict value=test
echo ""
echo ""

# Test 8: Conflict with subcommand - should execute subcommand
echo "Test 8: Conflict with subcommand - execute subcommand"
echo "Command: $APP conflict sub"
echo "---"
$APP conflict sub
echo ""
echo ""

# Test 9: Complex scenario - multiple formats mixed
echo "Test 9: Complex scenario - multiple formats mixed"
echo "Command: $APP complex pos1 pos2 flag1=value1 flag2=100"
echo "---"
$APP complex pos1 pos2 flag1=value1 flag2=100
echo ""
echo ""

# Test 10: Complex scenario - JSON + positional arguments
echo "Test 10: Complex scenario - JSON + positional arguments"
echo "Command: $APP complex pos1 '{\"flag1\":\"json-value\",\"flag2\":200}'"
echo "---"
$APP complex pos1 '{"flag1":"json-value","flag2":200}'
echo ""
echo ""

# Test 11: Test when argument has same name as subcommand
echo "Test 11: Argument with same name as subcommand"
echo "Command: $APP conflict sub arg1 arg2"
echo "---"
$APP conflict sub arg1 arg2
echo ""
echo ""

# Test 12: Test multiple Query parameters
echo "Test 12: Multiple Query parameters"
echo "Command: $APP query \"name=Alice&age=25&tags=developer&tags=engineer\""
echo "---"
$APP query "name=Alice&age=25&tags=developer&tags=engineer"
echo ""
echo ""

# Test 13: Test Form parameters with quotes
echo "Test 13: Form parameters with quotes"
echo "Command: $APP form \"user='admin user' email=admin@test.com active=false\""
echo "---"
$APP form "user='admin user' email=admin@test.com active=false"
echo ""
echo ""

echo "=========================================="
echo "All tests completed"
echo "=========================================="
