# Python {{.Version}} and uv. uv copies from its cache: the cache (~/.cache) and the repo's virtual
# environments are on different volumes, where links don't work.
RUN {{mounts "stacks/python.sh"}}VERSION={{.Version}} sh /tmp/jw/python.sh
ENV UV_PYTHON_INSTALL_DIR=/opt/uv/python \
    UV_LINK_MODE=copy
