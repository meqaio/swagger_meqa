FROM python:3.6.3

RUN apt-get update

RUN pip install mqtag
RUN python -m spacy download en_core_web_md
RUN find /usr/local/lib/python3.6 -name oov_prob -exec sed -i '1 s/-[0-9.]*/-100000000/g' {} \;

RUN apt-get install -y vim
RUN apt-get install -y less

RUN mkdir /meqa
RUN mkdir /meqa/bin
RUN mkdir /meqa/data

COPY ./mqgo/bin/linux_amd64/* /meqa/bin/
