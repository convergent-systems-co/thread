# Winget

The manifest set under `ConvergentSystemsCo.Thread/0.1.1/` installs the signed-by-checksum Windows ZIP from the Thread GitHub release as a portable `thread` command.

To validate locally with the Windows Community Repository tooling:

```powershell
winget validate --manifest .\packaging\winget\ConvergentSystemsCo.Thread\0.1.1
```

To submit a new release, copy the three manifest files into the matching version directory, update `PackageVersion`, the release URL, and `InstallerSha256`, then submit that directory to [microsoft/winget-pkgs](https://github.com/microsoft/winget-pkgs) following its contribution instructions. The repository does not store a publishing token and the release workflow does not silently submit third-party package-manager PRs.

The current manifest covers Windows x64. Add ARM64 only when a corresponding signed release artifact exists.
