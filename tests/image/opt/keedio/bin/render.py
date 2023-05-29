#!/usr/bin/env python
import argparse
from os import path
from os import environ
from jinja2 import Template

parser = argparse.ArgumentParser(description='Render utils')

parser.add_argument('-t', action='store', dest='template',required=True)
parser.add_argument('-o', action='store', dest='output',required=True)
args = parser.parse_args()
args.template=path.abspath(args.template)
args.output=path.abspath(args.output)
err=''
if not path.isfile(args.template):
    err += args.template +" file doesn't exists\n"
if not path.isdir(path.dirname(args.output)):
    err += args.output +" directory doesn't exists"
if err != '':
    print err
    exit(1)
f=open(args.template,'r')
#with open(args.template,'r') as f:
template=Template(f.read())
f.close()
output=open(args.output,'w')
output.write(template.render(environ))
output.close()
