import click

from commands import hello, test

di = {
    'config': {
        'version': '0.1.0'
    }
}

plugin_instances = [
    hello.setup(di), 
    test.setup(di)
]

cli = click.CommandCollection(sources=[i['cli'] for i in plugin_instances])

if __name__ == '__main__':
    cli()