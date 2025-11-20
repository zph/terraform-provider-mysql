# GoReleaser GPG Code Signing Plan for GitHub Actions

## Current State

1. **GoReleaser Configuration**: `.goreleaser.yml` is configured to sign checksums using GPG
   - Expects `GPG_FINGERPRINT` environment variable
   - Uses `--batch` flag for non-interactive signing
   - Signs the SHA256 checksum file

2. **GitHub Actions Status**: Release job is currently **DISABLED** (commented out)
   - Comment indicates: "DISABLED to figure out GPG signing issue on Github Actions"
   - Suspected issue: "possibly due to lack of TTY inside docker?"

3. **Existing Secrets**: There's a reference to `GPG_PRIVATE_KEY` secret in the Terraform Provider Release workflow

## Plan Overview

### Phase 1: GPG Subkey Creation and Export

**Objective**: Create a signing subkey from your main key and export it securely

**Why Subkeys?**
- Your main key stays secure on your local machine
- Subkeys can be exported for CI/CD use
- If a subkey is compromised, it can be revoked without affecting your main key
- Subkeys can have specific capabilities (signing only, encryption only, etc.)

1. **List Your Existing Keys**
   ```bash
   # List your secret keys to find your main key
   gpg --list-secret-keys --keyid-format LONG
   # Note the KEY_ID (the long hex string after "sec")
   ```

2. **Create a Signing Subkey**

   **Option A: Non-Interactive Method (Recommended - GPG 2.1+)**
   ```bash
   # Create a signing-only RSA 4096-bit subkey with no expiration
   # Replace YOUR_MAIN_KEY_ID with your actual main key ID
   gpg --quick-add-key YOUR_MAIN_KEY_ID rsa4096 sign 0
   
   # Or if you want it to expire in 2 years:
   # gpg --quick-add-key YOUR_MAIN_KEY_ID rsa4096 sign 2y
   
   # You'll be prompted for your main key passphrase
   ```

   **Option B: Interactive Method (if quick-add-key doesn't work)**
   ```bash
   # This requires an interactive terminal (not batch mode)
   # Make sure you're in a terminal with TTY access
   gpg --expert --edit-key YOUR_MAIN_KEY_ID
   
   # In the GPG prompt:
   # 1. Type: addkey
   # 2. Select: (4) RSA (sign only)
   # 3. Choose key size: 4096 (or your preference)
   # 4. Set expiration: 0 (no expiration) or a specific date
   # 5. Confirm: y
   # 6. Enter your main key passphrase
   # 7. Type: save
   ```

   **Note**: If you get "can't do this in batch mode" error, you need to:
   - Run it in an interactive terminal (not through a script)
   - Or use Option A with `--quick-add-key` which works in batch mode

3. **Export the Signing Subkey**
   ```bash
   # List keys again to see the new subkey
   gpg --list-secret-keys --keyid-format LONG YOUR_MAIN_KEY_ID
   # Look for the "ssb" line - that's your signing subkey
   # Note the SUBKEY_ID (the long hex string after "ssb")
   
   # Export ONLY the subkeys (not the main key)
   # This exports all subkeys of the main key, which is fine
   gpg --armor --export-secret-subkeys YOUR_MAIN_KEY_ID > gpg-signing-subkey.asc
   
   # Get the subkey fingerprint (this is what GoReleaser will use)
   # Use --with-subkey-fingerprints to show subkey fingerprints
   gpg --fingerprint --with-subkey-fingerprints YOUR_MAIN_KEY_ID
   
   # Or use --list-secret-keys with LONG format to see subkey IDs
   gpg --list-secret-keys --keyid-format LONG YOUR_MAIN_KEY_ID
   
   # The signing subkey will show as "ssb" with [S] flag
   # Example output:
   # ssb   rsa4096/ABC123DEF4567890 2024-01-01 [S] [expires: never]
   #       └─ This is the subkey ID (short form, 16 chars)
   # 
   # To get the full 40-character fingerprint of a specific subkey:
   gpg --fingerprint --with-subkey-fingerprints YOUR_MAIN_KEY_ID | grep -A 1 "\[S\]"
   # This will show the signing subkey with its full fingerprint
   ```

4. **Verify Subkey Export**
   ```bash
   # Verify only the subkey was exported (not the main key)
   gpg --list-packets gpg-signing-subkey.asc | grep -E "(keyid|secret)"
   # Should only show the subkey, not the main key
   ```

3. **Store in GitHub Secrets**
   - Add `GPG_PRIVATE_KEY`: The ASCII-armored subkey content from `gpg-signing-subkey.asc`
   - Add `GPG_FINGERPRINT`: The subkey fingerprint (40-character hex string)
   - Add `GPG_PASSPHRASE`: The passphrase for your main key (required for password-protected keys)
   
   **Important**: Store the subkey, not your main key! This way your main key stays secure.

### Phase 2: GitHub Actions Workflow Updates

**Objective**: Configure the release job to import and use GPG key

1. **Update `.github/workflows/main.yml`** - Uncomment and enhance the release job:

   ```yaml
   release:
     name: Release
     needs: [tests]
     if: ( startsWith( github.ref, 'refs/tags/v' ) ||
           startsWith(github.ref, 'refs/tags/v0.0.0-rc') )
     runs-on: ubuntu-22.04
     permissions:
       contents: write  # Required for creating releases
       id-token: write  # Required for OIDC if using
     steps:
       - name: Checkout Git repo
         uses: actions/checkout@v4
         with:
           fetch-depth: 0  # Full history needed for changelog
       
       - name: Set up Go
         uses: actions/setup-go@v4
         with:
           go-version-file: go.mod
       
       - name: Import GPG Subkey
         env:
           GPG_PRIVATE_KEY: ${{ secrets.GPG_PRIVATE_KEY }}
           GPG_FINGERPRINT: ${{ secrets.GPG_FINGERPRINT }}
           GPG_PASSPHRASE: ${{ secrets.GPG_PASSPHRASE }}
         run: |
           # Create GPG directory
           mkdir -p ~/.gnupg
           chmod 700 ~/.gnupg
           
           # Configure GPG for non-interactive use with passphrase
           echo "use-agent" >> ~/.gnupg/gpg.conf
           echo "pinentry-mode loopback" >> ~/.gnupg/gpg.conf
           echo "allow-loopback-pinentry" >> ~/.gnupg/gpg.conf
           
           # Start gpg-agent with loopback pinentry
           gpg-agent --daemon --allow-loopback-pinentry
           
           # Import the subkey
           echo "$GPG_PRIVATE_KEY" | gpg --batch --import --passphrase "$GPG_PASSPHRASE"
           
           # Trust the key (required for signing)
           # Use ultimate trust (6) for the subkey
           echo "$GPG_FINGERPRINT:6:" | gpg --import-ownertrust
           
           # Verify key is available and can sign
           gpg --list-secret-keys --keyid-format LONG
           
           # Test signing capability
           echo "test" | gpg --batch --pinentry-mode loopback --passphrase "$GPG_PASSPHRASE" --sign --armor
       
       - name: Run GoReleaser
         uses: goreleaser/goreleaser-action@v6
         with:
           distribution: goreleaser
           version: '~> v2'
           args: release --clean --skip=validate
         env:
           GITHUB_TOKEN: ${{ secrets.GITHUB_TOKEN }}
           GPG_FINGERPRINT: ${{ secrets.GPG_FINGERPRINT }}
           GPG_PASSPHRASE: ${{ secrets.GPG_PASSPHRASE }}
           GPG_TTY: $(tty)  # Ensure GPG can access TTY if needed
   ```

### Phase 3: GoReleaser Configuration Updates

**Objective**: Update `.goreleaser.yml` to sign all artifacts (not just checksums)

1. **Update Signing Configuration** in `.goreleaser.yml`:
   - Sign checksums (already configured)
   - Sign all binaries/archives (add this)
   - Add `--pinentry-mode loopback` for passphrase handling
   - Add `--passphrase` flag to use GPG_PASSPHRASE environment variable

2. **Updated `.goreleaser.yml` signing section**:
   ```yaml
   signs:
     - artifacts: checksum
       args:
         - "--batch"
         - "--pinentry-mode"
         - "loopback"
         - "--passphrase"
         - "{{ .Env.GPG_PASSPHRASE }}"
         - "--local-user"
         - "{{ .Env.GPG_FINGERPRINT }}"
         - "--output"
         - "${signature}"
         - "--detach-sign"
         - "${artifact}"
     - artifacts: archive
       args:
         - "--batch"
         - "--pinentry-mode"
         - "loopback"
         - "--passphrase"
         - "{{ .Env.GPG_PASSPHRASE }}"
         - "--local-user"
         - "{{ .Env.GPG_FINGERPRINT }}"
         - "--output"
         - "${signature}"
         - "--detach-sign"
         - "${artifact}"
   ```

3. **Alternative: Sign binaries directly** (if you want to sign the binaries themselves):
   ```yaml
   signs:
     - artifacts: binary
       args:
         - "--batch"
         - "--pinentry-mode"
         - "loopback"
         - "--passphrase"
         - "{{ .Env.GPG_PASSPHRASE }}"
         - "--local-user"
         - "{{ .Env.GPG_FINGERPRINT }}"
         - "--output"
         - "${signature}"
         - "--detach-sign"
         - "${artifact}"
     - artifacts: checksum
       # ... same as above
   ```

### Phase 4: Testing and Validation

**Objective**: Ensure the signing works correctly

1. **Test Workflow**:
   - Create a test tag (e.g., `v0.0.0-test`)
   - Push tag to trigger workflow
   - Verify release is created
   - Download and verify GPG signature:
     ```bash
     gpg --verify terraform-provider-mysql_VERSION_SHA256SUMS.sig terraform-provider-mysql_VERSION_SHA256SUMS
     ```

2. **Verify Signatures**:
   - Check that `.sig` files are created
   - Verify signatures are valid
   - Ensure public key can be found (consider uploading to keyserver or GitHub)

### Phase 5: Documentation Updates

**Objective**: Document the setup for future reference

1. **Update README.md**:
   - Add section on GitHub Actions GPG signing setup
   - Document required secrets
   - Add verification instructions

2. **Create Setup Guide** (optional):
   - Step-by-step guide for setting up GPG keys
   - Instructions for adding GitHub secrets
   - Troubleshooting section

## Security Considerations

1. **Key Management**:
   - ✅ **Use a subkey** (not your main key) - This is the recommended approach!
   - Your main key stays secure on your local machine
   - Only the signing subkey is exported and stored in GitHub Secrets
   - If the subkey is compromised, you can revoke it without affecting your main key
   - Rotate subkeys periodically (create new ones, revoke old ones)
   - Store subkey securely in GitHub Secrets (encrypted at rest)

2. **Subkey Benefits**:
   - Main key never leaves your machine
   - Subkeys can be scoped to specific capabilities (signing only)
   - Revocation is easier and doesn't affect your main identity
   - Can create multiple subkeys for different purposes

2. **Access Control**:
   - Limit who can modify GitHub Secrets
   - Use branch protection rules for release tags
   - Consider using GitHub Environments for additional protection

3. **Key Exposure**:
   - Never commit private keys to repository
   - Use GitHub Secrets (not environment variables in workflow files)
   - Consider using GitHub's OIDC for enhanced security

## Alternative Approaches

If GPG signing continues to be problematic:

1. **Cosign**: Use Sigstore/cosign for code signing (modern alternative)
2. **GitHub's Built-in Signing**: Use GitHub's automatic code signing (if available)
3. **Skip Signing**: Accept unsigned releases (not recommended for production)

## Implementation Checklist

- [ ] List existing GPG keys to find main key ID
- [ ] Create signing subkey from main key
- [ ] Export signing subkey (not main key!)
- [ ] Add `GPG_PRIVATE_KEY` secret to GitHub (subkey only)
- [ ] Add `GPG_FINGERPRINT` secret to GitHub (subkey fingerprint)
- [ ] Add `GPG_PASSPHRASE` secret to GitHub (main key passphrase)
- [ ] Update `.goreleaser.yml` to sign all artifacts
- [ ] Update GitHub Actions workflow with GPG import steps
- [ ] Test with a release tag
- [ ] Verify signatures work (checksums and archives)
- [ ] Update documentation
- [ ] Enable release job
- [ ] Test full release process

## References

- [GoReleaser Signing Documentation](https://goreleaser.com/customization/sign/)
- [GitHub Actions GPG Signing Guide](https://docs.github.com/en/actions/security-guides/encrypted-secrets)
- [GPG Batch Mode Documentation](https://www.gnupg.org/documentation/manuals/gnupg/GPG-Configuration.html)
