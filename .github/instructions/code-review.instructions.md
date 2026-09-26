---
applyTo: "**"
excludeAgent: "coding-agent"
---

When reviewing code, focus on:

## Security Critical Issues
- Check for hardcoded secrets, API keys, or credentials
- Look for SQL injection and XSS vulnerabilities
- Verify proper input validation and sanitization
- Review authentication and authorization logic

## Performance Red Flags
- Identify N+1 database query problems or inefficient finder methods on repositories
- Spot inefficient loops and algorithmic issues
- Check for memory leaks and resource cleanup
- Review caching opportunities for expensive operations

## Code Quality Essentials
- Functions should be focused and appropriately sized
- Use clear, descriptive naming conventions
- Ensure proper error handling throughout
- Identify code that is not fully implemented. This could be "TODO" markers in comments or wording that indicates a mock or non-production implementation. Other giveaways are phrases like "in a real implementation..." or "for now".
- Identify deviations from the architectural principles defined in `docs/ARCHITECTURE.md`
- Identify missing ADRs or violations of ADRs. ADRs are stored in the `adrs/` folder.

## Specification Adherence
- This repository uses specification-driven development. The constitution, located at `/.specify/memory/constitution.md`, is binding and must be adhered to.
- Specifications for the current branch can be found in the `specs/<PR branch name/` folder, the `spec.md` defines the "what", the `plan.md` the "how" of how a specfic feature is enabled. The `tasks.md` file must be fully reviewed before merging a PR. For every task that has been checked, verify that it is actually implemented with production-grade quality.  

## Review Style
- Be specific and actionable in feedback
- Explain the "why" behind recommendations
- Acknowledge good patterns when you see them
- Ask clarifying questions when code intent is unclear

Always prioritize security vulnerabilities and performance issues that could impact users.

Always suggest changes to improve readability. For example, this suggestion seeks to make the code more readable and also makes the validation logic reusable and testable.

// Instead of:
if (user.email && user.email.includes('@') && user.email.length > 5) {
  submitButton.enabled = true;
} else {
  submitButton.enabled = false;
}

// Consider:
function isValidEmail(email) {
  return email && email.includes('@') && email.length > 5;
}

submitButton.enabled = isValidEmail(user.email);
