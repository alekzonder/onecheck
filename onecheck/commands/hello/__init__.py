import click

def setup(di):
    @click.group()
    def cli():
        pass

    @cli.command('hello', help="hello command")
    @click.option('--name', required=True)
    def hello(name):
        version = di['config']['version']
        click.echo(f'[{version}] Hello {name}!')


    instance = {
        'cli': cli
    }

    return instance