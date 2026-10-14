`make agent` builds `jailwick-agent-linux-arm64` and `jailwick-agent-linux-amd64` here; they are
embedded into the jailwick binary and mounted read-only into every sandbox.

It also writes `agent-info.json` (`tools/agentinfo`): the jailwick release or build date and the
latest changes to the agent's code, from git. jailwick keeps it next to each agent it mounts, so a
pending restart can say in words what changed in the agent. It isn't in the agent itself: the
agent's bytes only change when its code does.
