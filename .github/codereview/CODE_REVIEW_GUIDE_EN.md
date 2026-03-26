# Code Review Comprehensive Guide

> A comprehensive code review guide combining best practices and categorized issue detection.
> 
> This document can be referenced by GitHub Copilot, review tools, and development teams.

## Table of Contents

- [Overview](#overview)
- [Golden Rules](#golden-rules)
- [Review Process Guidelines](#review-process-guidelines)
- [Issue Categories Quick Reference](#issue-categories-quick-reference)
- [Detailed Category Checklist](#detailed-category-checklist)
- [PR Submission Checklist](#pr-submission-checklist)
- [Usage Guide](#usage-guide)

---

## Overview

### Purpose

Code review serves two primary purposes:
1. **Find defects**: Testing typically catches 50-60% of issues, while well-executed code reviews can find 60-80% of defects.
2. **Improve quality**: Ensure software meets the six key characteristics: **Reliability, Efficiency, Usability, Maintainability, Security, and Portability**.

### Scope

When reviewing code, examine:
- Every line of code assigned for review
- Logic correctness
- Design decisions
- Code quality and maintainability

---

## Golden Rules

| # | Rule | Description |
|---|------|-------------|
| 1 | **Simple, Efficient, Secure Design** | Avoid over-engineering; prioritize clarity and security |
| 2 | **Strict Integrity & Performance** | Ensure data integrity and optimize where needed |
| 3 | **Proper Modulation** | High cohesion within modules, low coupling between them |
| 4 | **Minimize Repetition** | DRY principle - Don't Repeat Yourself |
| 5 | **Good Naming & Comments** | Clear, descriptive names; meaningful comments |

### Review Benchmark

> **Target: Find at least 1 logic issue per 100 lines of code (LOC)**

---

## Review Process Guidelines

### Before Review

1. Understand the context and requirements
2. Review related documentation and design documents
3. Check if tests are included

### During Review

1. Read the PR description thoroughly
2. Check all changed files systematically
3. Verify logic flow and edge cases
4. Look for patterns of issues (see categories below)
5. Consider security implications
6. Check for performance concerns

### After Review

1. Provide constructive feedback
2. Suggest specific improvements
3. Approve only when all critical issues are resolved

---

## Issue Categories Quick Reference

| Category | Code | Priority | Description |
|----------|------|----------|-------------|
| Requirements Unmatch | `REQ` | 🔴 Critical | Implementation doesn't match requirements |
| Logic Issues | `LOGI` | 🔴 Critical | Bugs preventing correct execution |
| Security | `SEC` | 🔴 Critical | Vulnerabilities and security risks |
| Authentication/Authorization | `AUTH` | 🔴 Critical | Access control issues |
| Design | `DSN` | 🟠 High | Architectural and design problems |
| Robustness | `RBST` | 🟠 High | Error handling and fault tolerance |
| Transaction | `TRANS` | 🟠 High | Database transaction issues |
| Concurrency | `CONC` | 🟠 High | Multi-threading problems |
| Performance | `PERF` | 🟠 High | Resource efficiency issues |
| Compatibility | `CPT` | 🟡 Medium | Version and environment compatibility |
| Idempotence | `IDE` | 🟡 Medium | Repeated operation safety |
| Maintainability | `MAIN` | 🟡 Medium | Long-term maintenance concerns |
| Coupling | `CPL` | 🟡 Medium | Module interdependencies |
| Readability | `READ` | 🟢 Normal | Code clarity issues |
| Simplicity | `SIMPL` | 🟢 Normal | Unnecessary complexity |
| Consistency | `CONS` | 🟢 Normal | Style and naming consistency |
| Duplication | `DUP` | 🟢 Normal | Repeated code/logic |
| Naming | `NAM` | 🟢 Normal | Variable/function naming |
| Doc String | `DOCS` | 🟢 Normal | Documentation comments |
| Comments | `COMM` | 🔵 Low | Inline comments |
| Logging | `LOGG` | 🔵 Low | Log statements |
| Error Messages | `ERR` | 🔵 Low | Error definitions |
| Format | `FOR` | 🔵 Low | Code formatting |
| Grammar | `GRAM` | 🔵 Low | Text grammar issues |
| Best Practice | `PRAC` | 🔵 Low | Convention violations |
| PR Description | `PR` | 🔵 Low | PR documentation |

---

## Detailed Category Checklist

### 🔴 Critical Issues

#### Requirements Unmatch (REQ)

**Definition**: The code implementation does not match the requirements/documentation.

**Checklist**:
- [ ] Code implementation matches requirements/documentation
- [ ] All specified behaviors are implemented
- [ ] Edge cases from requirements are handled
- [ ] Failure paths are considered, not just "happy paths"
- [ ] Assumptions are documented and validated

**Examples**:
```
[REQ] Documentation says A but implemented as B
[REQ] "Happy paths" are defined, but failure paths are ignored
[REQ] Requirement specifies 30s timeout, but code uses 10s
```

#### Logic Issues (LOGI)

**Definition**: Any problem preventing the software from running as expected.

**Checklist**:
- [ ] No null pointer dereferences
- [ ] No division by zero possibilities
- [ ] Array/list index bounds are checked
- [ ] No infinite recursion possibilities
- [ ] All if-else branches are complete
- [ ] Control flow is properly closed (if-else if has else)
- [ ] Form/input data is validated
- [ ] Loop termination conditions are correct

**Examples**:
```
[LOGI] Accessing user.name without checking if user is null
[LOGI] if-else if statement missing else branch for other cases
[LOGI] Array index accessed without bounds checking
[LOGI] Stack overflow due to infinite recursion
```

#### Security (SEC)

**Definition**: Prevent vulnerabilities and protect against threats like unauthorized access, data breaches, manipulation, or disruptions.

**Checklist**:
- [ ] No SQL injection vulnerabilities
- [ ] No XSS (Cross-site scripting) vulnerabilities
- [ ] No buffer overflow risks
- [ ] No hardcoded credentials or secrets
- [ ] Proper authentication checks
- [ ] No plaintext credentials in database
- [ ] Secure data transportation (HTTPS, encryption)
- [ ] Input validation on all user inputs
- [ ] Proper output encoding

**Examples**:
```
[SEC] SQL injection: query directly concatenates user input
[SEC] Hardcoded credentials: password = "admin123"
[SEC] XSS vulnerability: user input rendered without escaping
[SEC] Plaintext credentials stored in database
```

#### Authentication/Authorization (AUTH)

**Definition**: Issues related to identity verification and access control.

**Checklist**:
- [ ] Authentication is required where needed
- [ ] Authorization checks are in place
- [ ] Role-based access control is correctly implemented
- [ ] Session management is secure
- [ ] Tokens are properly validated

**Examples**:
```
[AUTH] API endpoint accessible without authentication
[AUTH] Admin functions lack role verification
[AUTH] JWT token not validated before use
```

### 🟠 High Priority Issues

#### Design (DSN)

**Definition**: The design/implementation should be simple, usable, secure, reliable, maintainable, scalable and efficient.

**Checklist**:
- [ ] Design is simple and straightforward
- [ ] Design is usable and intuitive
- [ ] Design is secure by default
- [ ] Design is reliable
- [ ] Design is maintainable
- [ ] Design is scalable
- [ ] Design is efficient
- [ ] Workflow is not overly complicated

**Examples**:
```
[DSN] Design is too complex: workflow has unnecessary steps
[DSN] Design is not efficient: processes data multiple times
[DSN] Design is not flexible: hard to extend for new requirements
```

#### Robustness (RBST)

**Definition**: Robustness refers to the ability of a system to handle errors, unexpected inputs, or stressful conditions gracefully without crashing.

**Checklist**:
- [ ] Exceptions are caught and handled appropriately
- [ ] Invalid data is prevented from affecting the system
- [ ] System continues operating despite component failures
- [ ] System can recover to a stable state after failures
- [ ] Graceful degradation is implemented where appropriate

**Examples**:
```
[RBST] Exception not caught: application crashes on network error
[RBST] Invalid input not validated: causes downstream errors
[RBST] No fallback mechanism when external service is unavailable
```

#### Transaction (TRANS)

**Definition**: A transaction is a sequence of operations performed as a single logical unit of work.

**Checklist**:
- [ ] Transaction boundaries are properly defined
- [ ] Transactions are rolled back on exceptions
- [ ] Long-running transactions are avoided
- [ ] Deadlock prevention measures are in place
- [ ] Transactions are used when needed for data integrity

**Examples**:
```
[TRANS] Missing transaction boundaries for multi-step operations
[TRANS] Transaction not rolled back on exception
[TRANS] Long-running transaction blocking other operations
[TRANS] Deadlock risk due to inconsistent lock ordering
```

#### Concurrency (CONC)

**Definition**: Issues that occur specifically in multithreading/multitask environments.

**Checklist**:
- [ ] No race conditions
- [ ] No deadlock possibilities
- [ ] Proper use of locks
- [ ] Thread-safe data structures where needed
- [ ] Atomic operations where required

**Examples**:
```
[CONC] Race condition: shared counter modified without synchronization
[CONC] Deadlock: methods A and B acquire locks in different order
[CONC] Thread-unsafe collection used in multi-threaded context
```

#### Performance (PERF)

**Definition**: Unnecessary consumption of excessive resources such as CPU, memory, disk, network, etc.

**Checklist**:
- [ ] Efficient algorithms are used (check Big O complexity)
- [ ] No excessive memory usage
- [ ] Database queries are optimized
- [ ] Batch operations used instead of one-by-one processing
- [ ] Proper indexing for database queries
- [ ] No unnecessary repeated operations
- [ ] Caching is used where appropriate
- [ ] No N+1 query problems

**Examples**:
```
[PERF] O(n²) algorithm used where O(n log n) is possible
[PERF] Database query inside loop: should use batch query
[PERF] Missing index for frequently filtered column
[PERF] N+1 query: loading related entities one by one
```

### 🟡 Medium Priority Issues

#### Compatibility (CPT)

**Definition**: Conflicts that prevent software from interacting properly with different versions or environments.

**Checklist**:
- [ ] Backward compatibility is maintained
- [ ] Forward compatibility is considered
- [ ] API changes are versioned appropriately
- [ ] Database schema changes are migration-safe
- [ ] Browser/OS compatibility is tested
- [ ] Library versions are compatible

**Examples**:
```
[CPT] API field removal breaks existing clients
[CPT] New feature not compatible with older browser versions
[CPT] Library upgrade introduces breaking changes
```

#### Idempotence (IDE)

**Definition**: Running an operation multiple times produces the same outcome.

**Checklist**:
- [ ] Repeated operations produce the same result
- [ ] No duplicate records on retry
- [ ] Delete operations handle already-deleted cases
- [ ] Payment/critical operations have idempotency keys

**Examples**:
```
[IDE] Placing same order twice incurs duplicate records
[IDE] Deletion for the second time returns error
[IDE] No idempotency key for payment operation
```

#### Maintainability (MAIN)

**Definition**: The degree to which an application can be understood, repaired, or enhanced. ~75% of project cost is maintenance!

**Checklist**:
- [ ] Code has good readability
- [ ] Code is modular with logical separation
- [ ] Complexity is kept low (no deep nesting)
- [ ] Code is testable
- [ ] Documentation is up-to-date
- [ ] Consistent coding patterns used

**Examples**:
```
[MAIN] Code is tightly coupled: changes require modifying multiple files
[MAIN] No unit tests: difficult to verify changes
[MAIN] Outdated documentation: doesn't reflect current behavior
```

#### Coupling (CPL)

**Definition**: The degree of interdependence between software modules. Low coupling and high cohesion is desired.

**Checklist**:
- [ ] No hardcoded dependencies
- [ ] Lower layers don't depend on higher layers
- [ ] Components depend on interfaces, not implementations
- [ ] Related logic is consolidated in one place
- [ ] No hidden dependencies (must call A before B)
- [ ] Data is passed via defined interfaces, not shared structures

**Examples**:
```
[CPL] Method receives entire User object but only uses userId
[CPL] Must call init() before start(), but dependency is not documented
[CPL] Business logic scattered across multiple unrelated modules
```

### 🟢 Normal Priority Issues

#### Readability (READ)

**Definition**: Clear code structure, naming conventions, and documentation.

**Checklist**:
- [ ] Variable names are descriptive (not x1, temp, val2)
- [ ] Functions are reasonably sized (not 100+ lines)
- [ ] Nesting is limited (max 3-4 levels)
- [ ] Proper indentation and spacing
- [ ] Simple code preferred over clever tricks
- [ ] Clear separation of logic into functions/modules

**Examples**:
```
[READ] Variable name 'd' is unclear, should be 'data' or more specific
[READ] Function has 150+ lines, should be split into smaller functions
[READ] 5 levels of nested if statements, should be refactored
```

#### Simplicity (SIMPL)

**Definition**: Design and implementation should be as simple as possible, avoiding unnecessary complexity.

**Checklist**:
- [ ] Logic is straightforward and easy to follow
- [ ] Each function/class has a single responsibility
- [ ] No over-engineering or speculative features
- [ ] Unused code and comments are removed
- [ ] No unnecessarily generic designs

**Examples**:
```
[SIMPL] Overly generic solution for a simple problem
[SIMPL] Unused helper function should be removed
[SIMPL] Premature optimization makes code hard to understand
```

#### Consistency (CONS)

**Definition**: Ensure consistency in documentation, naming, format, logic, comments, logging, etc.

**Checklist**:
- [ ] Consistent naming conventions (camelCase, snake_case, etc.)
- [ ] Consistent language in comments
- [ ] Code and comments are in sync
- [ ] Code and documentation are in sync
- [ ] Terminology is consistent throughout

**Examples**:
```
[CONS] Mixing camelCase and snake_case randomly
[CONS] Code changed but comment still describes old behavior
[CONS] Different terms used for same concept in code and docs
```

#### Duplication (DUP)

**Definition**: Duplicate code/logic in different places causes high coupling and low maintainability.

**Checklist**:
- [ ] No copy-paste code blocks
- [ ] Repeated expressions extracted to variables
- [ ] Common logic extracted to functions
- [ ] Shared code in appropriate utilities/helpers

**Examples**:
```
[DUP] Same validation logic copy-pasted in 3 different places
[DUP] Repeated expression cameras[i].getStream().getResolution() should be extracted to variable
```

#### Naming (NAM)

**Definition**: Names should be clear, descriptive but concise, and easy to understand.

**Checklist**:
- [ ] Names are clear and descriptive
- [ ] Names are concise (not more than 5 words)
- [ ] No vague words like "data", "info", "stuff"
- [ ] Arrays/lists have plural names (cameras, cameraList)
- [ ] Methods use verbs, classes use nouns
- [ ] Boolean variables are named as questions (isActive, hasPermission)

**Examples**:
```
[NAM] Variable 'tp' is too cryptic, should be 'timeoutPeriod'
[NAM] Array 'camera' should be 'cameras' or 'cameraList'
[NAM] Method 'calculation()' should be 'calculate()'
```

#### Doc String (DOCS)

**Definition**: Special comments that explain what a function, class, or module does.

**Checklist**:
- [ ] Functions/classes have docstrings when necessary
- [ ] Parameters are documented
- [ ] Return values are documented
- [ ] Exceptions/side effects are documented
- [ ] Docstrings are up-to-date with code

**Examples**:
```
[DOCS] Public API method missing docstring
[DOCS] Parameter 'options' not documented
[DOCS] Return value type and meaning not specified
```

### 🔵 Low Priority Issues

#### Comments (COMM)

**Definition**: Comments should be added as necessary to help with review and maintenance.

**Checklist**:
- [ ] Complex logic has explanatory comments
- [ ] Workarounds/hacks have context comments
- [ ] No commented-out dead code
- [ ] Comments are clear and concise
- [ ] Performance trade-offs are explained

**Examples**:
```
[COMM] Complex algorithm lacks explanation comments
[COMM] Workaround lacks context: why is this needed?
[COMM] Commented-out code should be removed
```

#### Logging (LOGG)

**Definition**: Log as needed and avoid unnecessary or excessive logging.

**Checklist**:
- [ ] Error conditions are logged
- [ ] Logs include necessary context (IDs, types)
- [ ] No excessive debug logging in production
- [ ] Appropriate log levels are used
- [ ] Sensitive data is not logged

**Examples**:
```
[LOGG] Error case not logged
[LOGG] Log message lacks context: missing request ID
[LOGG] Debug logs left in production code
```

#### Error Messages (ERR)

**Definition**: Add or define errors as needed with clear, concise context.

**Checklist**:
- [ ] Error messages are specific and helpful
- [ ] Error codes/types are defined when needed
- [ ] Context is included in error messages
- [ ] Errors are logged appropriately

**Examples**:
```
[ERR] Generic message "Something went wrong" lacks context
[ERR] Missing error code for client-facing API
[ERR] Error message doesn't indicate what action to take
```

#### Format (FOR)

**Definition**: Includes coding style, format, and wording.

**Checklist**:
- [ ] No typos
- [ ] Consistent indentation
- [ ] No excessive blank lines
- [ ] Proper spacing around operators
- [ ] Code matches team style guide

**Examples**:
```
[FOR] Typo in variable name: 'recieve' should be 'receive'
[FOR] Inconsistent indentation: mixing tabs and spaces
[FOR] Excessive blank lines between statements
```

#### Grammar (GRAM)

**Definition**: Comments, errors, and docs should be grammatically correct.

**Checklist**:
- [ ] Comments are grammatically correct
- [ ] Error messages are properly written
- [ ] Documentation sentences are complete

**Examples**:
```
[GRAM] Comment has typo: "the user is login" should be "the user is logged in"
[GRAM] Incomplete sentence in documentation
```

#### Best Practice (PRAC)

**Definition**: Conforms to programming style, conventions, common use cases, and team-agreed rules.

**Checklist**:
- [ ] Event handlers follow naming conventions (onXxx)
- [ ] Files are organized by feature/module
- [ ] Folder structure follows conventions
- [ ] No confusing or misleading patterns

**Examples**:
```
[PRAC] Event handler 'click' should be named 'onClick'
[PRAC] Utility file in wrong folder
[PRAC] Misleading variable name causes confusion
```

#### PR Description (PR)

**Definition**: The PR description should conform to team guidelines.

**Checklist**:
- [ ] PR description is clear and concise
- [ ] Design document linked (if applicable)
- [ ] Related tickets/issues linked
- [ ] API changes documented
- [ ] Test points listed
- [ ] Breaking changes highlighted

**Examples**:
```
[PR] Missing link to design document
[PR] No test points described
[PR] Breaking change not highlighted
```

---

## PR Submission Checklist

Before submitting a PR, ensure:

1. [ ] **Code follows guidelines**: good naming, high cohesion, low coupling, minimal repetition
2. [ ] **Self-review completed**: diff is as small as possible
3. [ ] **Complex code is commented**: particularly in hard-to-understand areas
4. [ ] **Logging includes context**: ID, reqId, etc.
5. [ ] **PR description is complete**: all required links included
6. [ ] **Documentation is updated**: corresponding changes made
7. [ ] **Tests are added/updated**: prove fix is effective or feature works
8. [ ] **All tests pass locally**: new and existing tests
9. [ ] **Dependent changes are merged**: downstream modules updated

---

## Usage Guide

### For Reviewers

1. **Systematic Review**: Check items by category to avoid missing issues
2. **Prioritize**: Focus on Critical and High priority issues first
3. **Label Clearly**: Use category codes like `[LOGI]`, `[SEC]`
4. **Provide Suggestions**: Don't just point out problems, suggest improvements

### For GitHub Copilot

When assisting with code reviews:
1. Reference the category codes when identifying issues
2. Explain the issue and its category
3. Suggest specific fixes
4. Consider multiple categories for each code section

### For Developers

1. **Self-Check**: Use this checklist before submitting PRs
2. **Understand Categories**: Know the severity of different issue types
3. **Continuous Improvement**: Improve coding habits based on feedback

### For Teams

1. **Customize**: Adjust priorities and checks based on project needs
2. **Track Data**: Monitor issue frequency to identify improvement areas
3. **Training Material**: Use for onboarding and skill development

---

## References

- [Google Code Review Guidelines](https://google.github.io/eng-practices/review/reviewer/looking-for.html)
- Internal Code Guidelines
- Security Best Practices

---

*Last Updated: 2026-01-22*
