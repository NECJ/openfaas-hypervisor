/* SPDX-License-Identifier: BSD-3-Clause */
/*
 * Authors: Simon Kuenzer <simon.kuenzer@neclab.eu>
 *
 * Copyright (c) 2019, NEC Laboratories Europe GmbH, NEC Corporation.
 *                     All rights reserved.
 *
 * Redistribution and use in source and binary forms, with or without
 * modification, are permitted provided that the following conditions
 * are met:
 *
 * 1. Redistributions of source code must retain the above copyright
 *    notice, this list of conditions and the following disclaimer.
 * 2. Redistributions in binary form must reproduce the above copyright
 *    notice, this list of conditions and the following disclaimer in the
 *    documentation and/or other materials provided with the distribution.
 * 3. Neither the name of the copyright holder nor the names of its
 *    contributors may be used to endorse or promote products derived from
 *    this software without specific prior written permission.
 *
 * THIS SOFTWARE IS PROVIDED BY THE COPYRIGHT HOLDERS AND CONTRIBUTORS "AS IS"
 * AND ANY EXPRESS OR IMPLIED WARRANTIES, INCLUDING, BUT NOT LIMITED TO, THE
 * IMPLIED WARRANTIES OF MERCHANTABILITY AND FITNESS FOR A PARTICULAR PURPOSE
 * ARE DISCLAIMED. IN NO EVENT SHALL THE COPYRIGHT HOLDER OR CONTRIBUTORS BE
 * LIABLE FOR ANY DIRECT, INDIRECT, INCIDENTAL, SPECIAL, EXEMPLARY, OR
 * CONSEQUENTIAL DAMAGES (INCLUDING, BUT NOT LIMITED TO, PROCUREMENT OF
 * SUBSTITUTE GOODS OR SERVICES; LOSS OF USE, DATA, OR PROFITS; OR BUSINESS
 * INTERRUPTION) HOWEVER CAUSED AND ON ANY THEORY OF LIABILITY, WHETHER IN
 * CONTRACT, STRICT LIABILITY, OR TORT (INCLUDING NEGLIGENCE OR OTHERWISE)
 * ARISING IN ANY WAY OUT OF THE USE OF THIS SOFTWARE, EVEN IF ADVISED OF THE
 * POSSIBILITY OF SUCH DAMAGE.
 */

#include <stdio.h>
#include <string.h>
#include <sys/socket.h>
#include <arpa/inet.h>
#include <unistd.h>
#include <errno.h>
#include <stdlib.h>

#define HOST_PORT 8080
#define LISTEN_PORT 8080
static const char reply[] = "HTTP/1.1 200 OK\r\n" \
			    "Content-type: text/html\r\n" \
			    "Connection: close\r\n" \
			    "\r\n" \
			    "<html> \
					<head> \
						<title>FaaS Webpage</title> \
					</head> \
					<body> \
						<h1 style=\"font-family: Source Sans Pro, Helvetica, sans-serif\">Hello from a unikernel 👋</h1> \
						<p style=\"font-family: Source Sans Pro, Helvetica, sans-serif\">This webpage was served from a unikernel based FaaS platform.</p> \
						<br/> \
						<button onclick=\" \
							const Http = new XMLHttpRequest(); \
							const url='http://openfaas-gateway/function/fact'; \
							Http.open(\'GET\', url); \
							Http.send(); \
							Http.onreadystatechange = (e) => { \
								getElementById(\'fact\').innerHTML=Http.responseText \
							} \
						\">Click Me!</button> \
						<p id=\"fact\"></p> \
					</body> \
				</html>\n";

static const char readyMessage[] = "POST /ready HTTP/1.1\r\nHost: 8080\r\n\r\n";

#define BUFLEN 2048
static char recvbuf[BUFLEN];

void register_ready(char *ip) {
	printf("Registering as Ready!\n");
	int valread, client_fd;
    struct sockaddr_in serv_addr;
    char buffer[1024] = { 0 };
	printf("Opening socket: ");
    if ((client_fd = socket(AF_INET, SOCK_STREAM, 0)) < 0) {
		perror("Socket creation error: ");
        exit(-1);
    }
	printf("Done\n");

	printf("Init serv_addr: ");
    serv_addr.sin_family = AF_INET;
    serv_addr.sin_port = htons(HOST_PORT);
  
    // Convert IPv4 and IPv6 addresses from text to binary
    // form
    if (inet_pton(AF_INET, ip, &serv_addr.sin_addr)<= 0) {
		perror("Invalid address/ Address not supported: ");
        exit(-1);
    }
	printf("Done\n");
	
	printf("Concerting to hv: ");
    if (connect(client_fd, (struct sockaddr*)&serv_addr, sizeof(serv_addr)) < 0) {
		perror("Connection Failed: ");
        exit(-1);
    }
	printf("Done\n");
	printf("Registering as ready: ");
    if(send(client_fd, readyMessage, strlen(readyMessage), 0) == -1) {
		perror("Failed to send ready message: ");
        exit(-1);
	}
	printf("Done\n");
	printf("Waiting for response: ");
	if (read(client_fd, buffer, 1024) == -1) {
		perror("Error reading response: ");
        exit(-1);
    }
	printf("Done\n");

    // closing the connected socket
    close(client_fd);
}

int main(int argc, char *argv[])
{
	int rc = 0;
	int srv, client;
	ssize_t n;
	struct sockaddr_in srv_addr;

	printf("Open socket: ");
	srv = socket(AF_INET, SOCK_STREAM, 0);
	if (srv < 0) {
		perror("Failed to create socket: ");
		goto out;
	}
	printf("Done\n");

	srv_addr.sin_family = AF_INET;
	srv_addr.sin_addr.s_addr = INADDR_ANY;
	srv_addr.sin_port = htons(LISTEN_PORT);

	printf("Binding to port: ");
	rc = bind(srv, (struct sockaddr *) &srv_addr, sizeof(srv_addr));
	if (rc < 0) {
		perror("Failed to bind socket: ");
		goto out;
	}
	printf("Done\n");

	/* Accept one simultaneous connection */
	printf("Start listening on port: ");
	rc = listen(srv, 1);
	if (rc < 0) {
		perror("Failed to listen on socket: ");
		goto out;
	}
	printf("Done\n");

	// register as ready with hypervisor
	register_ready(argv[1]);

	printf("Listening on port %d...\n", LISTEN_PORT);
	while (1) {
		client = accept(srv, NULL, 0);
		if (client < 0) {
			perror("Failed to accept incoming connection: ");
			goto out;
		}

		/* Receive some bytes (ignore errors) */
		if (read(client, recvbuf, BUFLEN) == -1) {
			perror("Error reading response: ");
			exit(-1);
		}

		/* Send reply */
		n = write(client, reply, sizeof(reply) - 1);
		if (n < 0)
			perror("Failed to send a reply: ");
		else
			printf("Sent a reply\n");

		/* Close connection */
		close(client);
	}

out:
	printf("Exiting\n");
	return 0;
}