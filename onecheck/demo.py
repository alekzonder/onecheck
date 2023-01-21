import click

from commands import hello, test

cli = click.CommandCollection(sources=[hello.cli, test.cli])

if __name__ == '__main__':
    cli()