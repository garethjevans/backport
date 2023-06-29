ARG BUILDER_IMAGE=golang:1.20
ARG RUNTIME_IMAGE=alpine/git:2.40.1

FROM $BUILDER_IMAGE AS build

WORKDIR /app
COPY . .
RUN CGO_ENABLED=0 GOOS=linux go build -o /app/backport main.go

FROM $RUNTIME_IMAGE AS runtime

WORKDIR /
ENV PORT=8080
EXPOSE 8080
COPY --from=build /app/backport /backport

ENTRYPOINT [ "/backport" ] 
