FROM meqa/python

RUN apt-get update
RUN apt-get install -y vim
RUN apt-get install -y less

RUN mkdir /meqa
RUN mkdir /meqa/bin
RUN mkdir /meqa/data

COPY ./mqgo/bin/linux_amd64/* /meqa/bin/
COPY ./mqserver/*.py /meqa/bin/

RUN echo -100000000 > /usr/local/lib/python3.6/site-packages/en_core_web_md/en_core_web_md-1.2.1/vocab/oov_prob

EXPOSE 8888
