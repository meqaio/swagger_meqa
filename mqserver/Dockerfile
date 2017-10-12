FROM python:3.6.3

RUN apt-get update

RUN pip install -y ruamel.yaml
RUN pip install -y spacy
RUN python -m spacy download en_core_web_md
RUN echo -100000000 > /usr/local/lib/python3.6/site-packages/en_core_web_md/en_core_web_md-1.2.1/vocab/oov_prob

RUN apt-get install -y vim
RUN apt-get install -y less

RUN mkdir /meqa
RUN mkdir /meqa/bin
RUN mkdir /meqa/data

COPY ./mqgo/bin/linux_amd64/* /meqa/bin/
COPY ./mqserver/*.py /meqa/bin/
