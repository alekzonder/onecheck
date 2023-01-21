import click

@click.group()
def cli():
    pass

@cli.command('test', help="hello command")
@click.option('--test')
def test(test):
    click.echo(f'Test {test}!')
