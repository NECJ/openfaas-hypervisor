# openfaas-hypervisor

_(This service was built as part of my University of Manchester, Computer Science Bsc, Third Year Project: Exploring Unikernels for Serverless Computing.)_

This repository contains a service that works in conjuction with a [fork of faas-netes](https://github.com/NECJ/faas-netes). The original version of [OpenFaaS](https://github.com/openfaas/faas) executes function code within containers called [watchdogs](https://github.com/openfaas/of-watchdog). This service allowed me to execute function code within microVMs, gVisor-containers and unikernels so that I could compare the resourse utilisation and execution speed of the three.
