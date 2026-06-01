from manimlib import *


class Lesson(Scene):
    def construct(self):
        title = Text("Executable lesson preview")
        self.play(Write(title))
        self.wait(1)
