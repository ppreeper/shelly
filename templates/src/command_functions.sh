# helper functions available to all commands

safe_basename() {
  # POSIX-safe basename replacement
  case "$1" in
    */*) echo "${1##*/}" ;;
    *) echo "$1" ;;
  esac
}
