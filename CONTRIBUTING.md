# Contributing

Thanks for helping. Open an issue first for anything larger than a small fix, so the change fits the SDK's contract with gragas before you write it.

- Pull requests go to `main`. Keep one change to a pull request.
- Run `task pre-push` before you push. It runs vet, the 32-bit vet, the tests, the secret scan and the contract check. `task hooks` arms it as a git hook once per clone. Go and every tool the checks use are pinned in `gate/pins.env`; nothing is installed.
- A change to the contract (`contract/`, `wire/`) is made in [gragas](https://github.com/birdtable/gragas) first; this module holds a copy, and `contract/PIN` names the gragas commit it was copied at.
- Every package but `declarative` uses only the standard library; keep it so.
- Add a test with the change, and a fuzz seed where it reads a tool's output.

Report a security problem as [SECURITY.md](SECURITY.md) says, never in a public issue.
