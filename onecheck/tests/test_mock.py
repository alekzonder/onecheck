from unittest.mock import MagicMock
import pprint


def test_mock():
    pprint.pprint = MagicMock(return_value=3)
    assert pprint.pprint() == 3
