# Node.js {{.Version}}, corepack (pnpm, yarn), tsx and TypeScript.
RUN {{mounts "stacks/node.sh"}}VERSION={{.Version}} sh /tmp/jw/node.sh
ENV PATH=/opt/node/bin:$PATH \
    COREPACK_ENABLE_DOWNLOAD_PROMPT=0
