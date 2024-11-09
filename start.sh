docker build -t colnio/sampledb:v0.0.1 -f Dockerfile .
docker build -f Dockerfile_pg . -t colnio/sampledb_pg
docker compose up -d