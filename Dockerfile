FROM alpine:latest

RUN apk add --no-cache curl tar

RUN curl -L https://github.com/Thruqe/whatsrook/releases/download/v4.0.0/whatsrook-linux-amd64.tar.gz -o whatsrook.tar.gz \
    && tar -xzf whatsrook.tar.gz \
    && rm whatsrook.tar.gz

ENV PORT=3000
EXPOSE ${PORT}

ENTRYPOINT ["./whatsrook"]