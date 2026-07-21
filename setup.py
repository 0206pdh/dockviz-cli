from __future__ import annotations

import os
import shutil
from pathlib import Path

from setuptools import setup
from setuptools.command.build_py import build_py as _build_py

try:
    from wheel.bdist_wheel import bdist_wheel as _bdist_wheel
except ImportError as exc:  # pragma: no cover - build dependency issue
    raise RuntimeError("wheel must be installed to build dockviz wheels") from exc


class build_py(_build_py):
    def run(self) -> None:
        super().run()

        binary_env = os.environ.get("DOCKVIZ_BINARY")
        if not binary_env:
            raise RuntimeError("DOCKVIZ_BINARY must point to a prebuilt dockviz binary")

        source = Path(binary_env).resolve()
        if not source.is_file():
            raise RuntimeError(f"DOCKVIZ_BINARY does not exist: {source}")

        package_dir = Path(self.build_lib) / "dockviz_cli"
        package_dir.mkdir(parents=True, exist_ok=True)

        binary_name = "dockviz.exe" if source.suffix.lower() == ".exe" else "dockviz"
        target = package_dir / binary_name
        shutil.copy2(source, target)

        if binary_name == "dockviz":
            target.chmod(target.stat().st_mode | 0o755)


class bdist_wheel(_bdist_wheel):
    def finalize_options(self) -> None:
        super().finalize_options()
        self.root_is_pure = False

        plat_name = os.environ.get("DOCKVIZ_WHEEL_PLAT_NAME")
        if plat_name:
            self.plat_name_supplied = True
            self.plat_name = plat_name

    def get_tag(self) -> tuple[str, str, str]:
        plat_name = os.environ.get("DOCKVIZ_WHEEL_PLAT_NAME")
        if not plat_name:
            return super().get_tag()

        python_tag = os.environ.get("DOCKVIZ_WHEEL_PY_TAG", "py3")
        abi_tag = os.environ.get("DOCKVIZ_WHEEL_ABI_TAG", "none")
        return python_tag, abi_tag, plat_name


setup(
    name="dockviz",
    version=os.environ.get("DOCKVIZ_VERSION", "0.0.0.dev0").lstrip("v"),
    description="Real-time Docker environment dashboard for your terminal",
    long_description="Platform wheel packaging for the dockviz Go binary.",
    author="0206pdh",
    license="MIT",
    url="https://github.com/0206pdh/dockviz-cli",
    package_dir={"": "python_pkg"},
    packages=["dockviz_cli"],
    include_package_data=True,
    zip_safe=False,
    python_requires=">=3.9",
    entry_points={
        "console_scripts": [
            "dockviz=dockviz_cli._launcher:main",
        ],
    },
    cmdclass={
        "build_py": build_py,
        "bdist_wheel": bdist_wheel,
    },
    classifiers=[
        "License :: OSI Approved :: MIT License",
        "Operating System :: MacOS",
        "Operating System :: Microsoft :: Windows",
        "Operating System :: POSIX :: Linux",
        "Programming Language :: Python :: 3",
        "Programming Language :: Go",
        "Topic :: System :: Monitoring",
    ],
)
