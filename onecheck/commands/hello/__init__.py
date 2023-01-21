import click

@click.group()
def cli():
    pass

@cli.command('hello', help="hello command")
@click.option('--name', required=True)
def hello(name):
    click.echo(f'Hello {name}!')