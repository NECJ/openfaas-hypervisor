FROM ubuntu:20.04

# Install dependencies
RUN apt-get update
RUN apt-get install wget git fakeroot build-essential ncurses-dev xz-utils libssl-dev bc flex libelf-dev bison -y

# Add build script
RUN echo '#!/bin/sh\n\
# Download and extract kernel source files\n\
wget https://mirrors.edge.kernel.org/pub/linux/kernel/v5.x/linux-5.10.tar.xz\n\
tar xvf linux-5.10.tar.xz\n\
# Download kernel config file\n\
wget -O .config https://raw.githubusercontent.com/firecracker-microvm/firecracker/main/resources/guest_configs/microvm-kernel-x86_64-5.10.config\n\
cd linux-5.10/\n\
cp ../.config ./\n\
# Build kernel\n\
yes "" | make vmlinux -j8\n\
mv vmlinux /output\n\
' > /build.sh

run chmod +x /build.sh

CMD ["/build.sh"]