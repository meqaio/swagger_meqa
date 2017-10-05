# Meqa User Guide #

### Examples
docker run --net="host" -it --rm -v d:/src/autoapi/mqgo/meqa_data/:/meqa_data meqa/go generate -d /meqa_data -s /meqa_data/swagger.yaml
docker run --net="host" -it --rm -v d:/src/autoapi/mqgo/meqa_data/:/meqa_data meqa/go run -p /meqa_data/path.yaml -d /meqa_data -s /meqa_data/swagger_meqa.yaml