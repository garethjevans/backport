ARG BUILDER_IMAGE=golang:1.19
ARG RUNTIME_IMAGE=cgr.dev/chainguard/git

FROM $BUILDER_IMAGE AS build

    WORKDIR /app
    ADD . .
    RUN go build -o backport ./...


FROM $RUNTIME_IMAGE AS runtime

	ENV PORT=8080
	EXPOSE 8080
    COPY --from=build /app/backport /backport
    CMD [ "/backport" ]
