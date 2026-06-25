package physics

// Ball is the match ball. While owned it is dribbled along the ground; once
// kicked it is a free body with full 3D flight — gravity, Magnus curve from
// Spin, and a bounce on landing (see stepFreeBall). Spin is a signed side-spin
// magnitude that curves the ball laterally as it travels.
type Ball struct {
	Pos  Vec3
	Vel  Vec3
	Spin float64 // signed side-spin; curves the ball perpendicular to its travel
}
