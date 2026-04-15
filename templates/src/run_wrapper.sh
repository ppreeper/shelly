run() {
  normalize_input "$@"
  if [ $# -eq 0 ]; then
    %APP_NAME%_usage
    return 0
  fi
  cmd=$1; shift
  case "$cmd" in
    download) download_command "$@" ;;
    upload) upload_command "$@" ;;
    -h|--help) %APP_NAME%_usage ;;
    *) echo "Unknown command: $cmd"; %APP_NAME%_usage; exit 2 ;;
  esac
}
