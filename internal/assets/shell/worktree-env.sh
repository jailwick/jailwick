# jailwick: a command run in a worktree gets that worktree's own ports, databases and Redis
# database, whoever made it (jw wt, or Claude Code through jailwick's worktree hooks). Bash runs
# this file before every non-interactive command (BASH_ENV); elsewhere it does nothing.
case "$PWD/" in
*/.jailwick/worktrees/* | */.claude/worktrees/*)
	eval "$(/opt/jailwick/bin/jailwick-agent worktree-env 2>/dev/null)"
	;;
esac
