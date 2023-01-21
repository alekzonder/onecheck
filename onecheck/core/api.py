"""
onecheck API class
"""
from .config import Config


class Api:
    """
    onecheck Api class
    """

    _config: Config

    def get_config(self) -> Config:
        """
        return config
        """
        return self._config

    def set_config(self, config: Config) -> None:
        """
        set config
        """
        self._config = config
