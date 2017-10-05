import falcon
import os
import json
import datetime
import subprocess
from wsgiref import simple_server

from vocabulary import Vocabulary
from tag import SwaggerDoc

ErrOK = 0
ErrInvalidYaml = 1

MQKey = 'api_key'
MQVersion = 'version'
MQStatus = 'status'
MQSwagger = 'swagger'
MQPlan = 'plan'

MQDataDir = '/meqa/data'

import falcon

class SpecResource(object):
    def __init__(self, vocab):
        self.vocab = vocab

    def get_new_data_dir(self, api_key):
        datadir = MQDataDir + '/' + api_key + '/' + datetime.datetime.now().strftime("%y-%m-%d-%H.%M.%S")
        try:
            os.makedirs(datadir, exist_ok=True)
        except OSError:
            if not os.path.isdir(datadir):
                return None

        return datadir

    def on_post(self, req, resp):
        body = req.stream.read()
        if not body:
            raise falcon.HTTPBadRequest('Empty request body',
                                        'A valid JSON document is required.')

        try:
            obj = json.loads(body.decode('utf-8'))
        except (ValueError, UnicodeDecodeError):
            raise falcon.HTTPError(falcon.HTTP_753,
                                   'Malformed JSON',
                                   'Could not decode the request body. The '
                                   'JSON was incorrect or not encoded as '
                                   'UTF-8.')

        api_key = obj.get(MQKey)
        if api_key == None or len(api_key) == 0:
            raise falcon.HTTPUnauthorized("invalid api_key")

        if not MQSwagger in obj:
            raise falcon.HTTPBadRequest('no swagger in request body')

        swagger = SwaggerDoc(self.vocab, obj[MQSwagger])
        if swagger.doc == None:
            raise falcon.HTTPBadRequest('failed to parse the swagger file posted')

        # save the original swagger
        dirpath = self.get_new_data_dir(api_key)
        with open(dirpath + '/' + 'swagger.yaml', 'w') as f:
            f.write(obj[MQSwagger])

        tagged_swagger_path = dirpath + '/' + 'swagger_meqa.yaml'
        swagger.gather_words()
        swagger.add_tags()
        swagger.dump(tagged_swagger_path)

        result = subprocess.run(["/meqa/bin/mqgen", "-d", dirpath, "-s", tagged_swagger_path, "-a", "all"], stdout=subprocess.PIPE)

        if result.returncode != 0:
            raise falcon.HTTPBadRequest("failed to generate test plan", result.stdout)

        respObj = dict()

        try:
            with open(tagged_swagger_path, 'r') as f:
                respObj['swagger_meqa'] = f.read()

            plans = dict()
            algos = ['simple', 'object', 'path']
            for algo in algos:
                with open(dirpath + '/' + algo + '.yaml') as f:
                    plans[algo] = f.read()

            respObj['test_plans'] = plans
        except:
            raise falcon.HTTPBadRequest("failed to generate test plan", "missing meqa files")

        resp.media = respObj

def main():
    vocab = Vocabulary()
    api = falcon.API()
    api.add_route('/specs', SpecResource(vocab))

    print("starting simple server")
    httpd = simple_server.make_server('0.0.0.0', 8888, api)
    httpd.serve_forever()

if __name__ == "__main__":
    main()

'''
@hug.get('/version')
def version():
    return {MQStatus : ErrOK, MQVersion : 'v0.8'}

@hug.default_input_format("application/json")
@hug.post('/specs')
def specs(body):
    return body

def main():
    api = hug.API(__name__)

    api.run()

if __name__ == "__main__":
    main()
'''