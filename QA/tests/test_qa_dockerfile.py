from pathlib import Path


def test_fixture_dockerfile_includes_common_cv_runtime_libraries():
    dockerfile = Path(__file__).resolve().parents[1] / "docker" / "fixture.Dockerfile"
    contents = dockerfile.read_text(encoding="utf-8")

    for package in ("libgl1", "libglib2.0-0", "libsm6", "libxext6", "libxrender1"):
        assert package in contents
