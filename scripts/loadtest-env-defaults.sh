# Source this file after assigning loadtest_env_file. Values already exported by
# the caller win; the dotenv file supplies defaults only.
if [ -n "${loadtest_env_file:-}" ] && [ -f "$loadtest_env_file" ]; then
	while IFS= read -r loadtest_env_line || [ -n "$loadtest_env_line" ]; do
		case "$loadtest_env_line" in
			''|'#'*|[[:space:]]*) continue ;;
		esac
		loadtest_env_key=${loadtest_env_line%%=*}
		if [ "$loadtest_env_key" = "$loadtest_env_line" ]; then
			echo "Invalid dotenv line in $loadtest_env_file: $loadtest_env_line" >&2
			return 1
		fi
		case "$loadtest_env_key" in
			[!A-Za-z_]*|*[!A-Za-z0-9_]*|'')
				echo "Invalid dotenv key in $loadtest_env_file: $loadtest_env_key" >&2
				return 1
				;;
		esac
		eval "loadtest_env_is_set=\${$loadtest_env_key+x}"
		if [ "$loadtest_env_is_set" = x ]; then
			continue
		fi
		loadtest_env_value=${loadtest_env_line#*=}
		case "$loadtest_env_value" in
			\"*\") loadtest_env_value=${loadtest_env_value#\"}; loadtest_env_value=${loadtest_env_value%\"} ;;
			\'*\') loadtest_env_value=${loadtest_env_value#\'}; loadtest_env_value=${loadtest_env_value%\'} ;;
		esac
		export "$loadtest_env_key=$loadtest_env_value"
	done < "$loadtest_env_file"
fi
unset loadtest_env_line loadtest_env_key loadtest_env_value loadtest_env_is_set
