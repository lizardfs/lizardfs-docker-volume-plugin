FROM golang:1.14
COPY . /app/
WORKDIR /app
RUN go build

FROM ubuntu:focal
RUN apt update && \
 apt install lizardfs-client -y && \
 apt-get clean && \
 rm -rf /var/lib/apt/lists/*
COPY --from=0 /app/lizardfs-volume-plugin /usr/bin/
ENTRYPOINT ["lizardfs-volume-plugin"]
