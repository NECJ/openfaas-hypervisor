FROM ubuntu:20.04

RUN apt-get update
# RUN apt-get install binutils bc wget xz-utils make gcc flex bison libncurses-dev gawk flex bison openssl libssl-dev dkms libelf-dev libudev-dev libpci-dev libiberty-dev autoconf llvm -y 
RUN apt-get install wget git fakeroot build-essential ncurses-dev xz-utils libssl-dev bc flex libelf-dev bison -y
RUN wget https://mirrors.edge.kernel.org/pub/linux/kernel/v5.x/linux-5.10.tar.xz
RUN tar xvf linux-5.10.tar.xz
RUN wget -O .config https://raw.githubusercontent.com/firecracker-microvm/firecracker/main/resources/guest_configs/microvm-kernel-x86_64-5.10.config
# RUN mv microvm-kernel-x86_64-5.10.config .config
# RUN apt-get build-dep linux linux-image-$(uname -r)

COPY build-script.sh .

CMD ["/build-script.sh"]