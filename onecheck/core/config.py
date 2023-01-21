"""
config class
"""


class Config(object):
    """
    onecheck Config class
    """
    version: str

    def get_version(self) -> str:
        """
        return version
        """
        return self.version
