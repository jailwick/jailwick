# Ruby: the system's, with bundler. Gems install as the user into ~/.local, which outlives the
# container.
RUN {{mounts "stacks/ruby.sh"}}VERSION={{.Version}} sh /tmp/jw/ruby.sh
ENV GEM_HOME=${SANDBOX_HOME}/.local/share/gem \
    PATH=${SANDBOX_HOME}/.local/share/gem/bin:$PATH
