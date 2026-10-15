# shelly Documentation

Documentation for **shelly**, a POSIX `sh` CLI generator.

## Contents

| Document | Purpose |
|---|---|
| [`specification.md`](specification.md) | Normative specification of shelly's behavior. |
| [`commands.md`](commands.md) | Reference for the `shelly` command-line tool and the generated script. |

## The one-paragraph summary

shelly parses the argument vector **once**, in a single interleaved loop, and stores each
parsed value in its own global scalar (`--force` → `${force}`) because POSIX `sh` has no
associative arrays. The parse order and the resulting variable names are specified in
[`specification.md`](specification.md) §3 and §4.
