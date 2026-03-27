FROM heroiclabs/nakama-pluginbuilder:3.22.0 AS builder
WORKDIR /backend
COPY go.mod go.sum ./
RUN go mod download
COPY . .
RUN go build --trimpath --mod=readonly --buildmode=plugin -o backend.so .

FROM heroiclabs/nakama:3.22.0
COPY --from=builder /backend/backend.so /nakama/data/modules/
COPY local.yml /nakama/data/local.yml
