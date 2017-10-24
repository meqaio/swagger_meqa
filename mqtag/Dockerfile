FROM python:3.6.3

RUN apt-get update

RUN apt-get install -y vim
RUN apt-get install -y less

RUN pip install spacy
RUN python -m spacy download en_core_web_md
RUN find /usr/local/lib/python3.6 -name oov_prob -exec sed -i '1 s/-[0-9.]*/-100000000/g' {} \;
RUN pip install mqtag==0.6.6

COPY ./mqgo/bin/linux_amd64/* /usr/local/bin/
