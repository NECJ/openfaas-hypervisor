all: rootfs/server

rootfs/server: server.c
	gcc -static -O3 server.c -o rootfs/server

clean:
	rm rootfs/*