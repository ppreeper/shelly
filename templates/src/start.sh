case "$0" in
  */*) script_name=${0##*/} ;;
  *) script_name=$0 ;;
esac

if [ "$script_name" = "$(basename "$0")" ]; then
  command_line_args="$@"
  initialize
  run "$@"
fi
