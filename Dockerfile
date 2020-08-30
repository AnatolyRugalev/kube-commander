FROM golang:1.15-alpine as build

ENV CGO_ENABLED=0
COPY . /app
RUN cd /app \
  && go build -o /kubecom ./cmd/kubecom

FROM alpine:3.12
ARG KUBECTL_VERSION=1.18.5
ENV \
  TERM=xterm-256color \
  EDITOR=nano \
  PAGER=less \
  LOGPAGER="jq -c -R -r '. as \$line | try fromjson catch \$line'"
RUN apk add --no-cache ca-certificates curl jq nano \
  && curl -L "https://storage.googleapis.com/kubernetes-release/release/v${KUBECTL_VERSION}/bin/linux/amd64/kubectl" > /usr/local/bin/kubectl \
  && chmod +x /usr/local/bin/kubectl \
  && curl -L "https://raw.githubusercontent.com/scopatz/nanorc/master/yaml.nanorc" > /usr/share/nano/yaml.nanorc \
  && echo "include /usr/share/nano/yaml.nanorc" > /root/.nanorc
COPY --from=build /kubecom /usr/local/bin/kubecom
ENTRYPOINT ["kubecom"]
VOLUME ["/root/.kube"]
