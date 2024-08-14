FROM ubuntu

RUN apt-get update && apt-get install tzdata -y && apt-get clean
COPY fluent-bit-to-alertmanager /usr/bin/
CMD ["fluent-bit-to-alertmanager"]