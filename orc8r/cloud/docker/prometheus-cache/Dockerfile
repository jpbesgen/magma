FROM golang:1.11

ARG CACHE_FILES=go/services/metricsd/prometheus/prometheus-cache

COPY ${CACHE_FILES} /go/src/magma/orc8r/cloud/go/services/metricsd/prometheus/prometheus-cache

WORKDIR /go/src/magma/orc8r/cloud/go/services/metricsd/prometheus/prometheus-cache

RUN go get -d -v ./...

RUN go install -v ./...

EXPOSE 9091

CMD ["prometheus-cache"]
