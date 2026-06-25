package physics

// Ball is the match ball. This phase keeps it ground-constrained (Y pinned to
// BallRadius). Vertical motion — gravity, Magnus, bounce — arrives in a later
// phase; here the ball is either free (mild ground drag) or being dribbled.
type Ball struct {
	Pos Vec3
	Vel Vec3
}
