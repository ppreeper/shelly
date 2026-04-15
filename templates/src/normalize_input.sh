normalize_input() {
  newargs=""
  for arg in "$@"; do
    case "$arg" in
      --*=*) newargs="$newargs ${arg%%=*} ${arg#*=}" ;;
      *) newargs="$newargs $arg" ;;
    esac
  done
  set -- $newargs
}
