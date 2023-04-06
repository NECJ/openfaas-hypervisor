SUBDIRS := $(wildcard */.)

all: fs

fs: 
	for dir in $(SUBDIRS); do \
	$(MAKE) -C $$dir all; \
	done

clean:
	rm -f vmlinux
	for dir in $(SUBDIRS); do \
	$(MAKE) -C $$dir clean; \
	done