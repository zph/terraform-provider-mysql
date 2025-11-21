zph/terraform-provider-mysql

# Purpose

zph fork of terraform-provider-mysql exists for the following goals:
1. To design and trial TiDB integrations or patches before committing upstream
2. To validate design changes for the project before offering upstream.

Changes here are intended to be upstreamed to petoju's fork to avoid ecosystem
fragmentation. We will update this readme if those design choices change.

## Release Naming

zph fork will use release naming in the following form:

v3.0.62005

{petoju version}{sequential int of additional applied patches}

This indicates that the base is v3.0.62 from petoju, with modifications from zph
repo that are 5 patch sets added.

It allows for keeping patches alive on this fork until they land upstream and are
released there.

## Security / Chain of Custody

We sign releases with a GPG key currently using goreleaser locally on the personal
equipment of @ZPH. As the maintainer of this fork, I, @ZPH, attest that the builds
represent the exact SHA of the version control with no alterations. The credentials
are stored in a credential manager with layers of safeguards and no other individuals
have access.

The near term goal is to setup github actions to provide this guarantee
so that even if I were a malicious actor or coerced,
I could not introduce opaque security issues into binary releases.

In the meantime, I certify by my professional reputation and career as:
https://www.linkedin.com/in/zph/ that appropriate safeguards are being taken.

## Release Process

The release process uses a PR-based workflow through the `make release` command. This creates a release branch, tags the release, and pushes both to GitHub. GitHub Actions then automatically builds and creates the release when the tag is pushed.

### Prerequisites

Before running the release process, ensure you have the following set up:

1. **GitHub Authentication**: You need authentication configured for pushing to GitHub.
   - Either configure SSH keys for git push, or
   - Set up a GitHub Personal Access Token with appropriate permissions

2. **Required Tools**:
   - `git` - For version control operations
   - `make` - For running the release command

**Note**: GPG signing and release building are handled automatically by GitHub Actions CI/CD. You don't need to configure GPG keys or goreleaser locally for the PR-based workflow.

### Release Workflow (PR-Based)

The `make release` command implements a PR-based workflow:

1. **Version Check**: Reads the current version from the `VERSION` file and checks if a tag for that version already exists.

2. **Tag Management**: 
   - If the tag already exists, it automatically suggests the next version by incrementing the build number (e.g., `v3.0.62006` → `v3.0.62007`)
   - You can accept the suggestion or enter a custom tag
   - If the tag doesn't exist, it uses the version from the `VERSION` file

3. **Version File Update**: If a new version was determined, the `VERSION` file is automatically updated with the new version number.

4. **Default Branch Update**: Fetches and updates the default branch (`r3.0.62`) to ensure the release branch is created from the latest code.

5. **Release Branch Creation**: Creates a new branch named `release/v{VERSION}` (e.g., `release/v3.0.62007`) from the updated default branch.

6. **Version File Commit**: If the `VERSION` file was modified, it commits the change on the release branch with message "Update VERSION to {version}".

7. **Tag Creation**: Creates an annotated git tag with the version number on the release branch.

8. **Push to GitHub**: Pushes both the release branch and tag to GitHub.

9. **GitHub Actions**: When the tag is pushed, GitHub Actions automatically:
   - Builds binaries for all supported platforms (Linux, macOS, Windows, FreeBSD)
   - Creates archives (zip files) for each platform/architecture combination
   - Generates SHA256 checksums
   - Signs the checksum file with GPG (configured in CI/CD)
   - Creates a GitHub release

10. **Pull Request**: Create a pull request from the release branch (`release/v{VERSION}`) to the default branch (`r3.0.62`).

11. **Merge**: After CI completes successfully, merge the PR to complete the release.

### Usage

To create a release using the PR-based workflow:

```bash
make release
```

The process will guide you through each step with clear prompts. You can cancel at any point if needed.

**Note**: The command automatically updates the default branch (`r3.0.62`) before creating the release branch, so you can run it from any branch. The release branch will always be created from the latest version of the default branch.

### Example Release Session

```bash
$ make release
Updating default branch (r3.0.62) before creating release branch...
Fetching latest changes from origin...
Checking out default branch r3.0.62...
Default branch updated successfully.
Checking if tag v3.0.62006 already exists...
Tag v3.0.62006 already exists!
Current version from VERSION file: 3.0.62006
Most recent tag: v3.0.62006 (version: 3.0.62006)

Suggested next tag: v3.0.62007
Enter next tag (or press Enter to use v3.0.62007): 

Updating VERSION file from 3.0.62006 to 3.0.62007...
VERSION file updated.

=========================================
Release Summary:
  Tag: v3.0.62007
  Version: 3.0.62007
  Release Branch: release/v3.0.62007
  Target Branch: r3.0.62
=========================================

Do you want to create release branch and tag v3.0.62007? (yes/no): yes

Creating release branch release/v3.0.62007 from updated default branch (r3.0.62)...
Release branch created from updated r3.0.62.

Committing VERSION file change...
VERSION file change committed.

Creating annotated tag v3.0.62007...
Tag v3.0.62007 created successfully.

Pushing branch and tag to origin...
Branch and tag pushed successfully.

=========================================
Release PR Created Successfully!
=========================================

Next steps:
1. GitHub Actions will automatically build the release when the tag is pushed.
2. Create a pull request:
   - Source: release/v3.0.62007
   - Target: r3.0.62
   - URL: https://github.com/zph/terraform-provider-mysql/compare/r3.0.62...release/v3.0.62007
3. Wait for CI to complete the release build.
4. Review and merge the PR into r3.0.62 to complete the release.

To switch back to your previous branch, run:
  git checkout your-feature-branch
```

### Local Testing Workflow (`make release-local`)

For local testing and debugging, you can use `make release-local` which runs goreleaser locally:

```bash
make release-local
```

This command:
- Runs the same version detection and tag management logic as `make release`
- Creates tags and commits locally
- Runs `goreleaser` locally to build and create a GitHub release
- Requires GPG key setup (set `GPG_FINGERPRINT` environment variable)
- Requires `goreleaser` installed locally

**Use `make release-local` only for:**
- Testing release builds locally
- Debugging goreleaser configuration
- Creating releases without CI/CD (not recommended for production)

**For production releases, always use `make release` (PR-based workflow).**

### Troubleshooting

- **GitHub push fails**: Check your git credentials or SSH keys are configured correctly
- **Branch already exists**: If the release branch already exists, delete it first or use a different version
- **CI/CD build fails**: Check GitHub Actions logs for build errors. The release will be created automatically when the tag is pushed successfully
- **PR merge conflicts**: Resolve conflicts in the PR before merging. The tag and release are already created, so merging just updates the default branch

## Original Readme
Below is from petoju/terraform-provider-mysql:

**This repository is an unofficial fork**

The fork is mostly based of the official (now archived) repo.
The provider has also some extra changes and solves almost all the reported
issues.

I incorporated changes by winebarrel/terraform-provider-mysql. Because I found
out about that repo only after some work, PR cannot be created easily now.
Feel free to create a PR with my code to their repo or the other way around.


Terraform Provider
==================

Requirements
------------

-	[Terraform](https://www.terraform.io/downloads.html) 0.12.x
-	[Go](https://golang.org/doc/install) 1.17 (to build the provider plugin)

Usage
-----

For Terraform 0.12+ compatibility, the configuration should specify version 1.6 or higher:

```hcl
provider "mysql" {
  version = "~> 1.6"
}
```

Building The Provider
---------------------

If you want to reproduce a build (to verify my build confirms to sources),
download the provider of any version first and find the correct go version:
```
egrep -a -o 'go1[0-9\.]+' path_to_the_provider_binary
```

Clone the repository anywhere. Use `goreleaser` to build the packages for all architectures:
```
goreleaser build --clean
```

Files in dist should match whatever is provided. If they don't, consider reading
https://words.filippo.io/reproducing-go-binaries-byte-by-byte/ or open an issue here.


Using the provider
----------------------
## Fill in for each provider

Developing the Provider
---------------------------

If you wish to work on the provider, you'll first need [Go](http://www.golang.org) installed on your machine (version 1.17+ is *required*). You'll also need to correctly setup a [GOPATH](http://golang.org/doc/code.html#GOPATH), as well as adding `$GOPATH/bin` to your `$PATH`.

To compile the provider, run `make build`. This will build the provider and put the provider binary in the `$GOPATH/bin` directory.

```sh
$ make bin
...
$ $GOPATH/bin/terraform-provider-mysql
...
```
### Ensure local requirements are present:

1. Docker environment
2. mysql-client binary which can be installed on Mac with `brew install mysql-client@8.0`
   1. Then add it to your path OR `brew link mysql-client@8.0`

### Running tests

In order to test the provider, you can simply run `make test`.

```sh
$ make test
```

In order to run the full suite of Acceptance tests, run `make testacc`.

*Note:* Acceptance tests create real resources, and often cost money to run.

```sh
$ make testacc
```

If you want to run the Acceptance tests on your own machine with a MySQL in Docker:

```bash
make acceptance
# or to test only one mysql version:
make testversion8.0
```

### CI Testing Strategy

Our CI workflow tests against multiple TiDB versions (latest of each minor series) to ensure compatibility across different releases. To optimize cache performance and avoid timeouts, we cache each TiDB version separately rather than caching all versions together. This approach:

- Prevents cache upload timeouts (each cache is ~700-800MB instead of 4.5GB)
- Allows each test job to download and cache only the version it needs
- Shares the TiUP binary cache across all tests for efficiency
- Automatically cleans up unused caches after 7 days

This strategy balances test coverage with CI performance and reliability.
