from setuptools import setup, find_packages
from codecs import open
from os import path

here = path.abspath(path.dirname(__file__))

setup(
    name='mqtag',
    version='0.6.3',
    description='mqtag command',
    long_description='Testing meqanized - http://meqa.io',
    url='https://github.com/meqaio/swagger_meqa',
    author='Ying Xie',
    author_email='ying@meqa.io',
    license='MIT',

    classifiers=[
        'Development Status :: 3 - Alpha',
        'Intended Audience :: Developers',
        'Topic :: Software Development :: Testing',
        'License :: OSI Approved :: MIT License',
        'Programming Language :: Python :: 3',
        'Programming Language :: Python :: 3.5',
        'Programming Language :: Python :: 3.6',
    ],

    keywords='swagger REST API testing',
    py_modules=["tag", "vocabulary"],
    install_requires=['en_core_web_md', 'spacy', 'ruamel.yaml'],

    entry_points={
        'console_scripts': [
            'mqtag=tag:main',
        ],
    },
)
