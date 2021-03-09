#!/bin/sh

openssl genrsa -out ca.key 1024
openssl req -new -x509 -days 3650 -key ca.key -out ca.crt -subj "/CN=localhost"
openssl genrsa -out cert.key 1024
#openssl pkcs12 -keypbe PBE-SHA1-3DES -certpbe PBE-SHA1-3DES -export -in ca.crt -inkey ca.key -out my_pkcs12.pfx -name "TEST"