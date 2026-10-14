# Ruby stack: the distribution's Ruby, with bundler and what native gems need to build. Its version
# is the one the base image's Debian or Ubuntu has (3.1 on jailwick's Debian 12 base); another
# version comes with a base image of its own (ruby:3.3-bookworm).
# shellcheck source=../project/lib.sh
. /tmp/jw/lib.sh

[ "$VERSION" = system ] || jw_fail "Ruby $VERSION: the Ruby stack is the system's; for another version use a base image like ruby:$VERSION-bookworm"
jw_resolve_only && echo "apt: ruby-full ruby-bundler" && exit 0
jw_apt_install ruby-full ruby-bundler build-essential
