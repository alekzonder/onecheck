import click

from core import Api


def hello(api: Api, name: str) -> None:
    """
    print hello
    """
    version = api.get_config().get_version()
    print(f'[{version}] Hello {name}!')


def setup(api: Api):
    """
    setup hello plugin
    """
    @click.group()
    def cli():
        pass

    @cli.command('hello', help="hello command")
    @click.option('--name', required=True)
    def cmd(name):
        hello(api, name)
        # di.set_result()

    instance = {
        'cli': cli
    }

    return instance
