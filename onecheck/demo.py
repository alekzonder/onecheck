"""
demo
"""
import click

from commands import hello, test

from core import Config, Api

config = Config()
config.version = '0.1.0'

api = Api()
api.set_config(config)

plugin_instances = [
    hello.setup(api),
    test.setup(api)
]

cli = click.CommandCollection(sources=[i['cli'] for i in plugin_instances])

if __name__ == '__main__':
    cli()
