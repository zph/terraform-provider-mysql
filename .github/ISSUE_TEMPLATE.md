Hi there,

please follow these steps to ensure quick resolution of your issue.

The maintainer (petoju) manages this project in a free time and thus doesn't have plenty of time to resolve all the issues. With that mentioned, PRs are always welcome.

### Provider version
Run `terraform -v` to show the version. If you are not running the latest version of this provider, please upgrade because your issue may have already been fixed.

You can find the latest version mentioned here: https://registry.terraform.io/providers/petoju/mysql/latest

### MySQL version and settings
Provide us with your DB version and non-standard DB settings. Did you add or change some `sql_mode` or some other config? Mention it.

### Terraform Configuration Files
In the ideal case, provide narrowed-down reproducer of your case. This should be a complete module with config connecting to localhost demonstrating the issue.

If you can, provide also docker command-line or docker-compose file starting mysql, which lead to the issue. That will remove any ambiguity regarding versions of tools and so on.

```hcl
# Copy-paste your Terraform configurations here. In case it's too large, feel free to package it and post a link to it.
```

### Debug Output
**Careful** the debug output often contains credentials. Please review the output line by line before providing it to anyone.

Please provider a link to a GitHub Gist containing provider debug output. You can get it by running your failing action with environment `TF_LOG_PROVIDER=DEBUG`. Please do NOT paste the debug output in the issue; just paste a link to the Gist.

In case reproducing the issue needs multiple runs, provide more GitHub Gists.

### Panic Output
If Terraform produced a panic, please provide a link to a GitHub Gist containing the output of the `crash.log`.

### Expected Behavior
What should have happened?

### Actual Behavior
What actually happened?

### Steps to Reproduce
Please list the steps required to reproduce the issue, for example:
1. `terraform apply`

### Important Factoids
Are there anything atypical about your accounts that we should know? For example: Running in EC2 Classic? Custom version of OpenStack? Tight ACLs?

### References
Are there any other GitHub issues (open or closed) or Pull Requests that should be linked here? For example:
- GH-1234
