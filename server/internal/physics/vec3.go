package physics

import "math"

// Vec3 is a 3D vector. The world is 3D so the ball can move and collide in all
// axes later, but players and the ball are ground-constrained this phase, so
// most operations work on the X/Z plane with Y held fixed.
type Vec3 struct {
	X, Y, Z float64
}

func (v Vec3) Add(o Vec3) Vec3      { return Vec3{v.X + o.X, v.Y + o.Y, v.Z + o.Z} }
func (v Vec3) Sub(o Vec3) Vec3      { return Vec3{v.X - o.X, v.Y - o.Y, v.Z - o.Z} }
func (v Vec3) Scale(s float64) Vec3 { return Vec3{v.X * s, v.Y * s, v.Z * s} }

func (v Vec3) LengthSq() float64 { return v.X*v.X + v.Y*v.Y + v.Z*v.Z }
func (v Vec3) Length() float64   { return math.Sqrt(v.LengthSq()) }

// LengthXZ is the magnitude in the ground plane (ignores Y).
func (v Vec3) LengthXZ() float64 { return math.Hypot(v.X, v.Z) }

// NormalizedXZ returns the unit vector in the X/Z plane (Y forced to 0). Returns
// the zero vector when the X/Z magnitude is ~0, so callers can detect "no
// direction" rather than dividing by zero.
func (v Vec3) NormalizedXZ() Vec3 {
	l := v.LengthXZ()
	if l < epsilon {
		return Vec3{}
	}
	return Vec3{v.X / l, 0, v.Z / l}
}

// DotXZ is the dot product in the ground plane (ignores Y).
func (v Vec3) DotXZ(o Vec3) float64 { return v.X*o.X + v.Z*o.Z }
